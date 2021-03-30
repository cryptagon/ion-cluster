package cmd

import (
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/mitchellh/go-homedir"
	cluster "github.com/pion/ion-cluster/pkg"
	logr "github.com/pion/ion-sfu/pkg/logger"

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

	log = logr.New().WithName("cmd")
)

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVarP(&cfgFile, "config", "c", "", "config file (default is $HOME/.cobra.yaml)")
	rootCmd.PersistentFlags().Bool("viper", true, "use Viper for configuration")
	viper.BindPFlag("useViper", rootCmd.PersistentFlags().Lookup("viper"))
}

// Execute executes the root command.
func Execute() error {
	rand.Seed(time.Now().UTC().UnixNano())
	return rootCmd.Execute()
}

func bindConfigEnvs(iface interface{}, parts ...string) {
	ifv := reflect.ValueOf(iface)
	ift := reflect.TypeOf(iface)
	for i := 0; i < ift.NumField(); i++ {
		v := ifv.Field(i)
		t := ift.Field(i)
		name := strings.ToLower(t.Name)
		tv, ok := t.Tag.Lookup("mapstructure")
		if ok {
			name = tv
		}
		switch v.Kind() {
		case reflect.Struct:
			bindConfigEnvs(v.Interface(), append(parts, name)...)
		default:
			log.V(1).Info(fmt.Sprintf("BINDENV: %v", strings.Join(append(parts, name), ".")))
			viper.BindEnv(strings.Join(append(parts, name), "."))
		}
	}
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
			log.Error(err, "Error: %v")
			os.Exit(1)
		}

		// Search config in home directory with name ".cobra" (without extension).
		viper.AddConfigPath(home)
		viper.SetConfigName(".ioncluster")
	}

	if err := viper.ReadInConfig(); err == nil {
		fmt.Println("Using config file:", viper.ConfigFileUsed())
	}

	viper.SetEnvPrefix("ION")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	bindConfigEnvs(conf)

	err := viper.GetViper().Unmarshal(&conf)
	if err != nil {
		log.Error(err, "sfu config file loaded failed. %v\n", "cfg", cfgFile)
		os.Exit(1)
	}

	if len(conf.SFU.WebRTC.ICEPortRange) > 2 {
		log.Error(err, "config file %s loaded failed. range port must be [min,max]\n", cfgFile)
		os.Exit(1)
	}

	// if len(conf.SFU.WebRTC.ICEPortRange) != 0 && conf.SFU.WebRTC.ICEPortRange[1]-conf.SFU.WebRTC.ICEPortRange[0] < portRangeLimit {
	// 	log.Errorf("config file %s loaded failed. range port must be [min, max] and max - min >= %d\n", file, portRangeLimit)
	// 	os.Exit(1)
	// }
}
