package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/taliesins/runcontainer/runcontainer"
	"io/ioutil"
	"os"
)

// initCmd represents the init command
var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create runcontainer config",
	Long: `Generate runcontainer.yaml configuration file.`,
	Run: func(cmd *cobra.Command, args []string) {
		dockerConfigurationsFileName := ".runcontainer.json"
		dockerConfigurations := &runcontainer.DockerConfigs{
			DefaultProfile: "default",
			Configs: map[string]*runcontainer.DockerConfig {
				"default" : {
					Image:                "iac",
					ImageTag:             "latest",
					EntryPoint:           "/bin/bash",
					MountPoint:           "current_sources",
					DockerInteractive:    true,
					WithDockerMount:      true,
					WithCurrentUser:      true,
					MountHomeDirectory:   true,
					DockerOptions:        []string{},
					TempDirMountLocation: runcontainer.MountLocHost,
					Environment:          map[string]string{},
					RunBeforeCommands:    []string{},
					RunAfterCommands:     []string{},
				},
			},
		}

		dockerConfigurationsBytes, err := PrettyJson(dockerConfigurations)
		if err != nil {
			os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}

		err = ioutil.WriteFile(dockerConfigurationsFileName, dockerConfigurationsBytes.Bytes(), 0644)
		if err != nil {
			os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}

		fmt.Fprintf(os.Stdout, "Created %s\n", dockerConfigurationsFileName)
		os.Exit(1)
	},
}

const (
	empty = ""
	tab   = "\t"
)

func PrettyJson(data interface{}) (*bytes.Buffer, error) {
	buffer := new(bytes.Buffer)
	encoder := json.NewEncoder(buffer)
	encoder.SetIndent(empty, tab)

	err := encoder.Encode(data)
	if err != nil {
		return nil, err
	}
	return buffer, nil
}

func init() {
	rootCmd.AddCommand(initCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// initCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// initCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
