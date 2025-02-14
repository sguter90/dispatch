package main

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	log "github.com/sirupsen/logrus"
	prefixed "github.com/x-cray/logrus-prefixed-formatter"
)

var (
	buildVersion = "v0.9.0-dev"
	buildCommit  = ""
	buildDate    = ""
)

var cfgFile string

var showVersion bool
var debug bool
var check bool

var dispatch *Dispatch

func main() {
	Execute()
}

// RootCmd represents the base command when called without any subcommands
var RootCmd = &cobra.Command{
	Use:    "dispatch",
	Short:  "A mail forwarding API service",
	Long:   `Run a webserver that provides an json api for emails`,
	PreRun: preRun,
	Run:    run,
}

// Execute adds all child commands to the root command sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	RootCmd.SetHelpTemplate(fmt.Sprintf("%s\nVersion:\n  github.com/gesquive/dispatch %s\n",
		RootCmd.HelpTemplate(), buildVersion))
	if err := RootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(-1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	RootCmd.PersistentFlags().StringVar(&cfgFile, "config", "",
		"Path to a specific config file (default \"./config.yml\")")
	RootCmd.PersistentFlags().StringP("log-file", "l", "",
		"Path to log file (default \"/var/log/dispatch.log\")")
	RootCmd.PersistentFlags().StringP("target-dir", "t", "",
		"Path to target configs (default \"/etc/dispatch/targets-enabled\")")
	RootCmd.PersistentFlags().BoolVar(&check, "check", false,
		"Check the config for errors and exit")

	RootCmd.PersistentFlags().StringP("address", "a", "0.0.0.0",
		"The IP address to bind the web server too")
	RootCmd.PersistentFlags().IntP("port", "p", 2525,
		"The port to bind the webserver too")
	RootCmd.PersistentFlags().StringP("rate-limit", "r", "inf",
		"The rate limit at which to send emails in the format 'inf|<num>/<duration>'. "+
			"inf for infinite or 1/10s for 1 email per 10 seconds.")

	RootCmd.PersistentFlags().StringP("smtp-server", "x", "localhost",
		"The SMTP server to send email through")
	RootCmd.PersistentFlags().Uint32P("smtp-port", "o", 25,
		"The port to use for the SMTP server")
	RootCmd.PersistentFlags().StringP("smtp-username", "u", "",
		"Authenticate the SMTP server with this user")
	RootCmd.PersistentFlags().StringP("smtp-password", "w", "",
		"Authenticate the SMTP server with this password")

	RootCmd.PersistentFlags().String("target-name", "",
		"Target name for an optional target")
	RootCmd.PersistentFlags().String("target-auth-token", "",
		"Target auth token for an optional target")
	RootCmd.PersistentFlags().String("target-from-address", "",
		"Target from address for an optional target")
	RootCmd.PersistentFlags().StringSlice("target-to-address", []string{},
		"Target to address list for an optional target")

	RootCmd.PersistentFlags().BoolVar(&showVersion, "version", false,
		"Display the version info and exit")
	RootCmd.PersistentFlags().BoolVarP(&debug, "debug", "D", false,
		"Include debug statements in log output")
	RootCmd.PersistentFlags().MarkHidden("debug")

	viper.SetEnvPrefix("dispatch")
	viper.AutomaticEnv()
	viper.BindEnv("config")
	viper.BindEnv("log_file")
	viper.BindEnv("target_dir")
	viper.BindEnv("address")
	viper.BindEnv("port")
	viper.BindEnv("rate_limit")
	viper.BindEnv("smtp_server")
	viper.BindEnv("smtp_port")
	viper.BindEnv("smtp_username")
	viper.BindEnv("smtp_password")
	viper.BindEnv("target_name")
	viper.BindEnv("target_auth_token")
	viper.BindEnv("target_from_address")
	viper.BindEnv("target_to_address")

	viper.BindPFlag("config", RootCmd.PersistentFlags().Lookup("config"))
	viper.BindPFlag("log_file", RootCmd.PersistentFlags().Lookup("log-file"))
	viper.BindPFlag("target_dir", RootCmd.PersistentFlags().Lookup("target-dir"))
	viper.BindPFlag("web.address", RootCmd.PersistentFlags().Lookup("address"))
	viper.BindPFlag("web.port", RootCmd.PersistentFlags().Lookup("port"))
	viper.BindPFlag("rate_limit", RootCmd.PersistentFlags().Lookup("rate-limit"))
	viper.BindPFlag("smtp.server", RootCmd.PersistentFlags().Lookup("smtp-server"))
	viper.BindPFlag("smtp.port", RootCmd.PersistentFlags().Lookup("smtp-port"))
	viper.BindPFlag("smtp.username", RootCmd.PersistentFlags().Lookup("smtp-username"))
	viper.BindPFlag("smtp.password", RootCmd.PersistentFlags().Lookup("smtp-password"))

	viper.SetDefault("log_file", "/var/log/dispatch.log")
	viper.SetDefault("target_dir", "/etc/dispatch/targets-enabled")
	viper.SetDefault("web.address", "0.0.0.0")
	viper.SetDefault("web.port", 2525)
	viper.SetDefault("rate_limit", "inf")
	viper.SetDefault("smtp.server", "localhost")
	viper.SetDefault("smtp.port", 25)

	dotReplacer := strings.NewReplacer(".", "_")
	viper.SetEnvKeyReplacer(dotReplacer)
}

// initConfig reads in config file and ENV variables if set.
func initConfig() {
	cfgFile := viper.GetString("config")
	if cfgFile != "" { // enable ability to specify config file via flag
		viper.SetConfigFile(cfgFile)
	} else {
		viper.SetConfigName("config")                 // name of config file (without extension)
		viper.AddConfigPath(".")                      // add current directory as first search path
		viper.AddConfigPath("$HOME/.config/dispatch") // add home directory to search path
		viper.AddConfigPath("/etc/dispatch")          // add etc to search path
	}

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		if !showVersion {
			if !strings.Contains(err.Error(), "Not Found") {
				fmt.Printf("Error opening config: %s\n", err)
			}
		}
	}
}

func preRun(cmd *cobra.Command, args []string) {
	if showVersion {
		fmt.Printf("github.com/gesquive/dispatch\n")
		fmt.Printf(" Version:    %s\n", buildVersion)
		if len(buildCommit) > 6 {
			fmt.Printf(" Git Commit: %s\n", buildCommit[:7])
		}
		if buildDate != "" {
			fmt.Printf(" Build Date: %s\n", buildDate)
		}
		fmt.Printf(" Go Version: %s\n", runtime.Version())
		fmt.Printf(" OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}
}

func run(cmd *cobra.Command, args []string) {
	log.SetFormatter(&prefixed.TextFormatter{
		TimestampFormat: time.RFC3339,
	})

	if debug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}

	log.Infof("running dispatch %s", buildVersion)
	if len(buildCommit) > 6 {
		log.Debugf("build: commit=%s", buildCommit[:7])
	}
	if buildDate != "" {
		log.Debugf("build: date=%s", buildDate)
	}
	log.Debugf("build: info=%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH)

	logFilePath := viper.GetString("log_file")
	log.Debugf("config: log_file=%s", logFilePath)
	if strings.ToLower(logFilePath) == "stdout" || logFilePath == "-" || logFilePath == "" {
		log.SetOutput(os.Stdout)
	} else {
		logFilePath = getLogFilePath(logFilePath)
		logFile, err := os.OpenFile(logFilePath, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			log.Fatalf("error opening log file=%v", err)
		}
		defer logFile.Close()
		log.SetOutput(logFile)
	}

	log.Infof("config: file=%s", viper.ConfigFileUsed())
	if viper.ConfigFileUsed() == "" {
		log.Fatal("No config file found.")
	}

	smtpSettings := SMTPSettings{
		viper.GetString("smtp.server"),
		viper.GetInt("smtp.port"),
		viper.GetString("smtp.username"),
		viper.GetString("smtp.password"),
	}
	log.Debugf("config: smtp={Host:%s Port:%d UserName:%s}", smtpSettings.Host,
		smtpSettings.Port, smtpSettings.UserName)

	targetsDir := viper.Get("target_dir").(string)
	log.Debugf("config: targets=%s", targetsDir)
	dispatch = NewDispatch(targetsDir, smtpSettings)

	targetAuth := viper.GetString("target_auth_token")
	targetName := viper.GetString("target_name")
	targetFrom := viper.GetString("target_from_address")
	targetTo := viper.GetStringSlice("target_to_address")
	singleTarget := DispatchTarget{}
	if len(targetName) > 0 && len(targetAuth) > 0 && len(targetTo) > 0 {
		singleTarget.Name = targetName
		singleTarget.AuthToken = targetAuth
		singleTarget.To = targetTo
		if len(targetFrom) > 0 {
			singleTarget.From = targetFrom
		}
		log.Debugf("adding optional target: %v", singleTarget)
		dispatch.AddTarget(singleTarget)
	} else {
		log.Debugf("not enough info to add optional target")
	}

	address := viper.GetString("web.address")
	port := viper.GetInt("web.port")

	limitMax, limitTTL, err := getRateLimit(viper.GetString("rate_limit"))
	if err != nil {
		log.Fatalf("error parsing limit: %v", err)
	}

	if check {
		log.Debugf("config: webserver=%s:%d", address, port)
		log.Debugf("config: rate-limit=%1.1f/%s", limitMax, limitTTL)
		log.Infof("Config file format checks out, exiting")
		if !debug {
			log.Infof("Use the --debug flag for more info")
		}
		os.Exit(0)
	}

	// finally, run the webserver
	server := NewServer(dispatch, limitMax, limitTTL)
	server.Run(fmt.Sprintf("%s:%d", address, port))
}

func getRateLimit(rateLimit string) (limitMax float64, limitTTL time.Duration, err error) {
	if rateLimit == "inf" {
		return math.MaxFloat64, time.Nanosecond, nil
	}

	parts := strings.Split(rateLimit, "/")
	if len(parts) != 2 {
		msg := fmt.Sprintf("rate limit is not formatted properly - %v", rateLimit)
		return limitMax, limitTTL, errors.New(msg)
	}
	limitMax, err = strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return
	}
	limitTTL, err = time.ParseDuration(parts[1])
	if err != nil {
		return
	}
	return
}

func getLogFilePath(defaultPath string) (logPath string) {
	fi, err := os.Stat(defaultPath)
	if err == nil && fi.IsDir() {
		logPath = path.Join(defaultPath, "dispatch.log")
	} else {
		logPath = defaultPath
	}
	return
}
