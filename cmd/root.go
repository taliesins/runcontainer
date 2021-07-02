package cmd

import (
	"encoding/json"
	"fmt"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/taliesins/runcontainer/runcontainer"
	"io/ioutil"
	"os"
)

var cfgFile string


// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "runcontainer",
	Short: "Easily run container with volume and env variables mapped",
	Long: `
Map current path and home directory into container
Map env variables into container
`,

	// Uncomment the following line if your bare application
	// has an action associated with it:
	Run: func(cmd *cobra.Command, args []string) {

		// If a config file is found, read it in.
		if err := viper.ReadInConfig(); err == nil {
			fmt.Fprintln(os.Stdout, "Using config file:", viper.ConfigFileUsed())
		} else {
			fmt.Fprintln(os.Stderr, "Unable to find config file to use")
			os.Exit(1)
		}
		dockerConfigurationsFileName := viper.ConfigFileUsed()

		var dockerConfigurations runcontainer.DockerConfigs

		//ignore viper for actual value
		dockerConfigurationsBytes, err := ioutil.ReadFile(dockerConfigurationsFileName)
		if err != nil {
			os.Stderr.WriteString(err.Error())
			os.Exit(1)
		}

		err = json.Unmarshal(dockerConfigurationsBytes, &dockerConfigurations)
		if err != nil {
			fmt.Fprintln(os.Stderr, "unable to deserialize configuration file ", err)
			os.Exit(1)
		}

		profile, _:= cmd.Flags().GetString("profile")
		if profile == "" {
			profile = dockerConfigurations.DefaultProfile
		}

		if profile == "" {
			profile = "default"
		}

		dockerConfiguration := dockerConfigurations.Configs[profile]
		if dockerConfiguration.TempDirMountLocation == "" {
			dockerConfiguration.TempDirMountLocation = runcontainer.MountLocHost
		}
		if dockerConfiguration.DockerOptions == nil {
			dockerConfiguration.DockerOptions = []string{}
		}
		if dockerConfiguration.RunBeforeCommands == nil {
			dockerConfiguration.RunBeforeCommands = []string{}
		}
		if dockerConfiguration.RunAfterCommands == nil {
			dockerConfiguration.RunAfterCommands = []string{}
		}
		if dockerConfiguration.Environment == nil {
			dockerConfiguration.Environment = map[string]string{}
		}
		if dockerConfiguration.MountPoint == "" {
			dockerConfiguration.MountPoint = "current_sources"
		}

		// Handle eventual panic message
		defer func() {
			if err := recover().(error); err != nil {
				os.Stderr.WriteString(err.Error())
				os.Exit(1)
			}
		}()

		os.Exit(dockerConfiguration.Execute())
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	cobra.CheckErr(rootCmd.Execute())
}

func init() {
	rootCmd.AddCommand(initCmd)

	cobra.OnInitialize(initConfig)

	// Here you will define your flags and configuration settings.
	// Cobra supports persistent flags, which, if defined here,
	// will be global for your application.

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is '$CWD/.runcontainer.json,$HOME/.runcontainer.json')")
	rootCmd.PersistentFlags().StringVar(&cfgFile, "profile", "", "profile to use (default is 'default')")
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
	} else {
		// Find in current directory.
		workingDirectory, err := os.Getwd()
		cobra.CheckErr(err)

		// Find home directory.
		home, err := os.UserHomeDir()
		cobra.CheckErr(err)

		// Search config in home directory with name ".runcontainer" (without extension).
		viper.AddConfigPath(workingDirectory)
		viper.AddConfigPath(home)
		viper.SetConfigType("json")
		viper.SetConfigName(".runcontainer")
	}
}
