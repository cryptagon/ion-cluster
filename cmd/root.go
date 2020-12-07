package cmd

import (
	"fmt"
	"os"

	"github.com/mitchellh/go-homedir"
	cluster "github.com/pion/ion-cluster/pkg"
	log "github.com/pion/ion-log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	// Used for flags.
	cfgFile string
	conf    = cluster.RootConfig{}

	rootCmd = &cobra.Command{
		Use:   "ion-cluster",
		Short: "ion-cluster is a fully featured and scalable webrtc sfu ",
		Long:  `A batteries included and scalable implementation of ion-sfu`,
	}
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default is $HOME/.cobra.yaml)")
	rootCmd.PersistentFlags().Bool("viper", true, "use Viper for configuration")
	viper.BindPFlag("useViper", rootCmd.PersistentFlags().Lookup("viper"))

	fixByFile := []string{"asm_amd64.s", "proc.go", "icegatherer.go"}
	fixByFunc := []string{}
	log.Init(conf.SFU.Log.Level, fixByFile, fixByFunc)
}

// Execute executes the root command.
func Execute() error {
	return rootCmd.Execute()
}

func initConfig() {
	if cfgFile != "" {
		// Use config file from the flag.
		viper.SetConfigFile(cfgFile)
		viper.SetConfigType("toml")
	} else {
		// Find home directory.
		home, err := homedir.Dir()
		if err != nil {
			log.Errorf("Error: %v", err)
			os.Exit(1)
		}

		// Search config in home directory with name ".cobra" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".ioncluster")
	}
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	err := viper.GetViper().Unmarshal(&conf)
	if err != nil {
		log.Errorf("sfu config file %s loaded failed. %v\n", cfgFile, err)

		os.Exit(1)
	}

	if len(conf.SFU.WebRTC.ICEPortRange) > 2 {
		log.Errorf("config file %s loaded failed. range port must be [min,max]\n", cfgFile)
		os.Exit(1)
	}

	// if len(conf.SFU.WebRTC.ICEPortRange) != 0 && conf.SFU.WebRTC.ICEPortRange[1]-conf.SFU.WebRTC.ICEPortRange[0] < portRangeLimit {
	// 	log.Errorf("config file %s loaded failed. range port must be [min, max] and max - min >= %d\n", file, portRangeLimit)
	// 	os.Exit(1)
	// }

	// if host := os.Getenv("ION_CLUSTER_HOST"); host != "" {
	// 	conf.Signal.FQDN = host
	// }
}
