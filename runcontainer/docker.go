package runcontainer

import (
	"bytes"
	"context"
	"fmt"
	"github.com/blang/semver"
	"github.com/coveooss/gotemplate/v3/utils"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

const (
	dockerSocketFile     = "/var/run/docker.sock"
	minimumDockerVersion     = "1.25"
	runContainerImageVersion = "RUNCONTAINER_IMAGE_VERSION"
	dockerMountImagePath     = "/var/runcontainer"
	dockerVolumeName         = "tgf"
)

// Version is initialized at build time through -ldflags "-X main.Version=<version number>"
var version = "(Locally Built)"

var dockerClient *client.Client
var dockerContext context.Context

// MountLocation is a docker mount location
type MountLocation string

// Mount locations
const (
	MountLocNone   MountLocation = "none"
	MountLocHost   MountLocation = "host"
	MountLocVolume MountLocation = "volume"
)

type DockerConfigs struct {
	DefaultProfile string 			`yaml:"default-profile,omitempty" json:"default-profile,omitempty" hcl:"default-profile,omitempty"`
	Configs map[string]*DockerConfig `yaml:"configs,omitempty" json:"configs,omitempty" hcl:"configs,omitempty"`
}

type DockerConfig struct{
	Image                string            `yaml:"docker-image,omitempty" json:"docker-image,omitempty" hcl:"docker-image,omitempty"`
	ImageTag             string            `yaml:"docker-image-tag,omitempty" json:"docker-image-tag,omitempty" hcl:"docker-image-tag,omitempty"`
	EntryPoint           string            `yaml:"entry-point,omitempty" json:"entry-point,omitempty" hcl:"entry-point,omitempty"`
	MountPoint           string            `yaml:"mount-point,omitempty" json:"mount-point,omitempty" hcl:"mount-point,omitempty"`
	DockerInteractive    bool              `yaml:"docker-interactive,omitempty" json:"docker-interactive,omitempty" hcl:"docker-interactive,omitempty"`
	WithDockerMount      bool              `yaml:"with-docker-mount,omitempty" json:"with-docker-mount,omitempty" hcl:"with-docker-mount,omitempty"`
	WithCurrentUser      bool              `yaml:"with-current-user,omitempty" json:"with-current-user,omitempty" hcl:"with-current-user,omitempty"`
	MountHomeDirectory   bool              `yaml:"mount-home-directory,omitempty" json:"mount-home-directory,omitempty" hcl:"mount-home-directory,omitempty"`
	DockerOptions        []string          `yaml:"docker-options,omitempty" json:"docker-options,omitempty" hcl:"docker-options,omitempty"`
	TempDirMountLocation MountLocation     `yaml:"temp-dir-mount-location,omitempty" json:"temp-dir-mount-location,omitempty" hcl:"temp-dir-mount-location,omitempty"`
	Environment          map[string]string `yaml:"environment,omitempty" json:"environment,omitempty" hcl:"environment,omitempty"`
	RunBeforeCommands    []string          `yaml:"run-before-commands,omitempty" json:"run-before-commands,omitempty" hcl:"run-before-commands,omitempty"`
	RunAfterCommands     []string          `yaml:"run-after-commands,omitempty" json:"run-after-commands,omitempty" hcl:"run-after-commands,omitempty"`
}

func (config *DockerConfig) GetImageName() string {
	var suffix string
	if config.ImageTag != "" {
		suffix += config.ImageTag
	}
	if len(suffix) > 1 {
		return fmt.Sprintf("%s:%s", config.Image, suffix)
	}
	return config.Image
}

func (config *DockerConfig) Execute() int {
	cwd, err := getCwd()
	if err != nil {
		panic(err)
	}

	currentDrive := fmt.Sprintf("%s/", filepath.VolumeName(cwd))
	rootFolder := strings.Split(strings.TrimPrefix(cwd, currentDrive), "/")[0]
	sourceFolder := fmt.Sprintf("/%s", filepath.ToSlash(strings.Replace(strings.TrimPrefix(cwd, currentDrive), rootFolder, config.MountPoint, 1)))

	imageName := config.GetImageName()

	dockerArgs := []string{
		"run",
	}
	if config.DockerInteractive {
		dockerArgs = append(dockerArgs, "-it")
	}
	dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s%s:/%s", convertDrive(currentDrive), rootFolder, config.MountPoint), "-w", sourceFolder)

	if config.WithDockerMount {
		withDockerMountArgs := getDockerMountArgs()
		dockerArgs = append(dockerArgs, withDockerMountArgs...)
	}

	currentUser, err := user.Current()
	if err != nil {
		panic(err)
	}

	// No need to map to current user on windows. Files written by docker containers in windows seem to be accessible by the user calling docker
	if config.WithCurrentUser && runtime.GOOS != "windows" {
		dockerArgs = append(dockerArgs, fmt.Sprintf("--user=%s:%s", currentUser.Uid, currentUser.Gid))
	}

	if config.MountHomeDirectory {
		home := filepath.ToSlash(currentUser.HomeDir)
		mountingHome := fmt.Sprintf("/home/%s", filepath.Base(home))

		dockerArgs = append(dockerArgs, []string{
			"-v", fmt.Sprintf("%v:%v", convertDrive(home), mountingHome),
			"-e", fmt.Sprintf("HOME=%v", mountingHome),
		}...)
	} else if config.TempDirMountLocation != MountLocNone {
		// If temp location is not disabled, we persist the home folder in a docker volume
		imageSummary, err := getImageSummary(imageName)
		if err != nil {
			panic(err)
		}

		image, err := inspectImage(imageSummary.ID)
		if err != nil {
			panic(err)
		}

		username := currentUser.Username

		if image.Config.User != "" {
			// If an explicit user is defined in the image, we use that user instead of the actual one
			// This ensure to not mount a folder with no permission to write into it
			username = image.Config.User
		}

		// Fix for Windows containing the domain name in the Username (e.g. ACME\jsmith)
		// The backslash is not accepted for a Docker volume path
		splitUsername := strings.Split(username, "\\")
		username = splitUsername[len(splitUsername) - 1]

		homePath := fmt.Sprintf("/home/%s", username)
		dockerArgs = append(dockerArgs,
			"-e", fmt.Sprintf("HOME=%s", homePath),
			"-v", fmt.Sprintf("%s-%s:%s", dockerVolumeName, username, homePath),
		)
	}

	dockerArgs = append(dockerArgs, config.DockerOptions...)

	switch config.TempDirMountLocation {
	case MountLocHost:
		tempDir, err := filepath.EvalSymlinks(os.TempDir())
		if err != nil {
			panic(err)
		}

		temp := filepath.ToSlash(filepath.Join(tempDir, "runcontainer-cache"))
		tempDrive := fmt.Sprintf("%s/", filepath.VolumeName(temp))
		tempFolder := strings.TrimPrefix(temp, tempDrive)
		if runtime.GOOS == "windows" {
			os.Mkdir(temp, 0755)
		}
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s%s:%s", convertDrive(tempDrive), tempFolder, dockerMountImagePath))
		config.Environment["RUNCONTAINER_TEMP_FOLDER"] = path.Join(tempDrive, tempFolder)
	case MountLocNone:
		// Nothing to do
	case MountLocVolume:
		// docker's -v option will automatically create the volume if it doesn't already exist
		dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:%s", dockerVolumeName, dockerMountImagePath))
	default:
		// We added a mount location and forgot to handle it...
		panic(fmt.Sprintf("Unknown mount location '%s'.  Please report a bug.", config.TempDirMountLocation))
	}

	config.Environment["RUNCONTAINER_COMMAND"] = config.EntryPoint
	config.Environment["RUNCONTAINER_VERSION"] = version
	config.Environment["RUNCONTAINER_ARGS"] = strings.Join(os.Args, " ")
	config.Environment["RUNCONTAINER_LAUNCH_FOLDER"] = sourceFolder
	config.Environment["RUNCONTAINER_IMAGE_NAME"] = imageName // sha256 of image
	config.Environment["RUNCONTAINER_IMAGE"] = config.Image
	if config.ImageTag != "" {
		config.Environment["RUNCONTAINER_IMAGE_TAG"] = config.ImageTag
	}

	if len(config.Environment) > 0 {
		for key, val := range config.Environment {
			os.Setenv(key, val)
		}
	}

	for _, do := range config.DockerOptions {
		dockerArgs = append(dockerArgs, strings.Split(do, " ")...)
	}

	if !listContainsElement(dockerArgs, "--name") {
		// We do not remove the image after execution if a name has been provided
		dockerArgs = append(dockerArgs, "--rm")
	}

	command := append(strings.Split(config.EntryPoint, " "))

	dockerArgs = append(dockerArgs, getEnviron(config.MountHomeDirectory)...)
	dockerArgs = append(dockerArgs, imageName)
	dockerArgs = append(dockerArgs, command...)

	dockerCmd := exec.Command("docker", dockerArgs...)
	dockerCmd.Stdin = os.Stdin
	dockerCmd.Stdout = os.Stdout

	var stderr bytes.Buffer
	dockerCmd.Stderr = &stderr

	if err := runCommands(config.RunBeforeCommands); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("run before command failed: %v\r\n%v", config.RunBeforeCommands, err))
		return 1
	}
	if err := dockerCmd.Run(); err != nil {
		if stderr.Len() > 0 {
			os.Stderr.WriteString(fmt.Sprintf("%s\n%s %s", stderr.String(), dockerCmd.Args[0], strings.Join(dockerArgs, " ")))
			if runtime.GOOS == "windows" {
				os.Stderr.WriteString(windowsMessage)
			}
			return 1
		}
	}
	if err := runCommands(config.RunAfterCommands); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("run after command failed: %v\r\n%v", config.RunBeforeCommands, err))
		return 1
	}

	return dockerCmd.ProcessState.Sys().(syscall.WaitStatus).ExitStatus()
}

var windowsMessage = `
You may have to share your drives with your Docker virtual machine to make them accessible.
On Windows 10+ using Hyper-V to run Docker, simply right click on Docker icon in your tray and
choose "Settings", then go to "Shared Drives" and enable the share for the drives you want to 
be accessible to your dockers.
On previous version using VirtualBox, start the VirtualBox application and add shared drives
for all drives you want to make shareable with your dockers.
IMPORTANT, to make your drives accessible to tgf, you have to give them uppercase name corresponding
to the drive letter:
	C:\ ==> /C
	D:\ ==> /D
	...
	Z:\ ==> /Z
`

func getCwd() (string, error){
	cwd, err := os.Getwd()
	if err != nil {
		return "", nil
	}

	cwd, err = filepath.EvalSymlinks(cwd)
	if err != nil {
		return "", nil
	}

	cwd = filepath.ToSlash(cwd)

	return cwd, nil
}

func getDockerClient() (*client.Client, context.Context, error) {
	if dockerClient == nil {
		os.Setenv("DOCKER_API_VERSION", minimumDockerVersion)
		newDockerClient, err := client.NewEnvClient()
		if err != nil {
			return nil, nil, err
		}
		dockerClient = newDockerClient
		dockerContext = context.Background()
	}
	return dockerClient, dockerContext, nil
}

// Returns the image name to use
// If docker-image-build option has been set, an image is dynamically built and the resulting image digest is returned
func (docker *DockerConfig) getImage() (name string) {
	name = docker.GetImageName()
	if !strings.Contains(name, ":") {
		name += ":latest"
	}

	return
}

// GetActualImageVersion returns the real image version stored in the environment variable TGF_IMAGE_VERSION
func (config *DockerConfig) GetActualImageVersion() (string, error) {
	return getActualImageVersionInternal(config.getImage())
}

func getImageSummary(imageName string) (*types.ImageSummary, error) {
	cli, ctx, err := getDockerClient()
	if err != nil {
		return nil, err
	}
	// Find image
	filters := filters.NewArgs()
	filters.Add("reference", imageName)
	images, err := cli.ImageList(ctx, types.ImageListOptions{Filters: filters})
	if err != nil {
		return nil, err
	}

	if len(images) != 1 {
		return nil, nil
	}

	return &images[0], nil
}

func inspectImage(imageID string) (types.ImageInspect, error) {
	cli, ctx, err := getDockerClient()
	if err != nil {
		return types.ImageInspect{}, err
	}
	inspect, _, err := cli.ImageInspectWithRaw(ctx, imageID)
	if err != nil {
		return types.ImageInspect{}, err
	}
	return inspect, nil
}

func getActualImageVersionFromImageID(imageID string) (string, error) {
	inspect, err := inspectImage(imageID)
	if err != nil {
		return "", err
	}
	for _, v := range inspect.ContainerConfig.Env {
		values := strings.SplitN(v, "=", 2)
		if values[0] == runContainerImageVersion {
			return values[1], nil
		}
	}
	// We do not found an environment variable with the version in the images
	return "", nil
}

func getActualImageVersionInternal(imageName string) (string, error) {
	image, err := getImageSummary(imageName)
	if err != nil {
		return "", err
	}

	if image != nil {
		return getActualImageVersionFromImageID(image.ID)
	}
	return "", nil
}

func getImageHash(imageName string) (string, error) {
	image, err := getImageSummary(imageName)
	if err != nil {
		return "", err
	}

	if image != nil {
		return image.Labels["hash"], nil
	}
	return "", nil
}

func checkImage(image string) bool {
	var out bytes.Buffer
	dockerCmd := exec.Command("docker", []string{"images", "-q", image}...)
	dockerCmd.Stdout = &out
	dockerCmd.Run()
	return out.String() != ""
}

func getDockerUpdateCmd(image string) *exec.Cmd {
	dockerUpdateCmd := exec.Command("docker", "pull", image)
	dockerUpdateCmd.Stdout, dockerUpdateCmd.Stderr = os.Stderr, os.Stderr
	return dockerUpdateCmd
}

func deleteImage(id string) (error) {
	cli, ctx, err := getDockerClient()
	if err != nil {
		return err
	}
	items, err := cli.ImageRemove(ctx, id, types.ImageRemoveOptions{})
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Untagged != "" {
			os.Stdout.WriteString(fmt.Sprintf("Untagged %s\n", item.Untagged))
		}
		if item.Deleted != "" {
			os.Stdout.WriteString(fmt.Sprintf("Deleted %s\n", item.Deleted))
		}
	}
	return nil
}

func CheckVersionRange(version, compare string) (bool, error) {
	if strings.Count(version, ".") == 1 {
		version = version + ".9999" // Patch is irrelevant if major and minor are OK
	}
	v, err := semver.Make(version)
	if err != nil {
		return false, err
	}

	comp, err := semver.ParseRange(compare)
	if err != nil {
		return false, err
	}

	return comp(v), nil
}

var reVersion = regexp.MustCompile(`(?P<version>\d+\.\d+(?:\.\d+){0,1})`)
var reVersionWithEndMarkers = regexp.MustCompile(`^` + reVersion.String() + `$`)

// https://regex101.com/r/ZKt4OP/5
var reImage = regexp.MustCompile(`^(?P<image>.*?)(?::(?:` + reVersion.String() + `(?:(?P<sep>[\.-])(?P<spec>.+))?|(?P<fix>.+)))?$`)

func (config *DockerConfig) prune(images ...string) error{
	cli, ctx, err := getDockerClient()
	if err != nil {
		return err
	}
	if len(images) > 0 {
		actualImageVersion, err := config.GetActualImageVersion()
		if err != nil {
			return err
		}
		current := fmt.Sprintf(">=%s", actualImageVersion)
		for _, image := range images {
			filters := filters.NewArgs()
			filters.Add("reference", image)
			if images, err := cli.ImageList(ctx, types.ImageListOptions{Filters: filters}); err == nil {
				for _, image := range images {
					actual, err := getActualImageVersionFromImageID(image.ID)
					if err != nil {
						return err
					}
					if actual == "" {
						for _, tag := range image.RepoTags {
							matches, _ := multiMatch(tag, reImage)
							if version := matches["version"]; version != "" {
								if len(version) > len(actual) {
									actual = version
								}
							}
						}
					}
					upToDate, err := CheckVersionRange(actual, current)
					if err != nil {
						os.Stderr.WriteString(fmt.Sprintf("Check version for %s vs%s: %v", actual, current, err))
					} else if !upToDate {
						for _, tag := range image.RepoTags {
							deleteImage(tag)
						}
					}
				}
			}
		}
	}
	cli, ctx, err = getDockerClient()
	if err != nil {
		return err
	}
	danglingFilters := filters.NewArgs()
	danglingFilters.Add("dangling", "true")
	if _, err := cli.ImagesPrune(ctx, danglingFilters); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Error pruning dangling images (Untagged):", err))
	}
	if _, err := cli.ContainersPrune(ctx, filters.Args{}); err != nil {
		os.Stderr.WriteString(fmt.Sprintf("Error pruning unused containers:", err))
	}

	return nil
}

func getEnviron(noHome bool) (result []string) {
	for _, env := range os.Environ() {
		split := strings.Split(env, "=")
		varName := strings.TrimSpace(split[0])
		varUpper := strings.ToUpper(varName)

		if varName == "" || strings.Contains(varUpper, "PATH") && strings.HasPrefix(split[1], string(os.PathSeparator)) {
			// We exclude path variables that actually point to local host folders
			continue
		}

		if runtime.GOOS == "windows" {
			if strings.Contains(strings.ToUpper(split[1]), `C:\`) || strings.Contains(strings.ToUpper(split[1]), `D:\`) || strings.Contains(strings.ToUpper(split[1]), `E:\`)   || strings.Contains(varUpper, "WIN") {
				continue
			}
		}

		switch varName {
		case
			"_", "PWD", "PS1", "OLDPWD", "TMPDIR",
			"PROMPT", "SHELL", "SH", "ZSH", "HOME",
			"LANG", "LC_CTYPE", "DISPLAY", "TERM":
		default:
			result = append(result, "-e")
			result = append(result, split[0])
		}
	}
	return
}

// This function set the path converter function
// For old Windows version still using docker-machine and VirtualBox,
// it transforms the C:\ to /C/.
func getPathConversionFunction() func(string) string {
	if runtime.GOOS != "windows" || os.Getenv("DOCKER_MACHINE_NAME") == "" {
		return func(path string) string { return path }
	}

	return func(path string) string {
		return fmt.Sprintf("/%s%s", strings.ToUpper(path[:1]), path[2:])
	}
}
var convertDrive = getPathConversionFunction()

func runCommands(commands []string) error {
	for _, script := range commands {
		cmd, tempFile, err := utils.GetCommandFromString(script)
		if err != nil {
			return err
		}
		if tempFile != "" {
			defer func() { os.Remove(tempFile) }()
		}
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func listContainsElement(list []string, element string) bool {
	for _, item := range list {
		if item == element {
			return true
		}
	}

	return false
}

// MultiMatch returns a map of matching elements from a list of regular expressions (returning the first matching element).
func multiMatch(s string, expressions ...*regexp.Regexp) (map[string]string, int) {
	for exprIndex, re := range expressions {
		if matches := re.FindStringSubmatch(s); len(matches) != 0 {
			results := make(map[string]string, len(matches))
			results[""] = matches[0]
			for subIndex, key := range re.SubexpNames() {
				if key != "" {
					results[key] = matches[subIndex]
				}
			}
			return results, exprIndex
		}
	}
	return nil, -1
}