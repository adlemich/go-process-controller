package main

import (
	"flag"
	"fmt"
	"gpcconfig"
	"gpclogging"
	"gpcprocessmgr"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

//#######################################################
//### GLOBAL VARIABLES, INIT, CONSTS
//#######################################################
func initProg() {

}

// const strings
const (
	GPCVersion = "0.2"
	GPCAuthor  = "Michael Adler"
	// Default config file
	GPCDefConfigFile = "./pc-conf.json"
)

//#########################################################
//#########################################################
func printHelp() {
	fmt.Println("############################################################")
	fmt.Println("# Process Controller v", GPCVersion, ". Written by", GPCAuthor)
	fmt.Println("# This tool helps you start, monitor and control processes.")
	fmt.Println("# ")
	fmt.Println("# Arguments:")
	fmt.Println("# ")
	fmt.Println("#   -h")
	fmt.Println("#       Prints this help output")
	fmt.Println("#   -cf <path to file>")
	fmt.Println("#       Path to the configuration file. Must be in JSON format. Default is", GPCDefConfigFile)
	fmt.Println("#   -dc <path to file>")
	fmt.Println("#       Creates a new default configuration file with the specified file name")
	fmt.Println("############################################################")
}

//#########################################################
//#########################################################
func main() {

	// First thing is to register a catch of the Crtl-C and kill events
	sigs := make(chan os.Signal, 1)
	appEnd := make(chan bool, 1)
	var shutdownWaitGroup sync.WaitGroup

	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		signal := <-sigs
		fmt.Println("Shutdown request received:", signal)
		gpcprocessmgr.ShutdownAll()
		appEnd <- true
	}()

	// ---- Local Variables
	var bCmdFlagH bool
	var sCmdFlagCF string
	var sCmdFlagDC string

	// SETUP CMD LINE ARGUMENTS
	flag.BoolVar(&bCmdFlagH, "h", false, "Prints help output")
	flag.StringVar(&sCmdFlagCF, "cf", GPCDefConfigFile, "Path to the configuration file. Must be in JSON format.")
	flag.StringVar(&sCmdFlagDC, "dc", "", "Creates a new default configuration file with the specified file name")
	flag.Parse()

	if bCmdFlagH {
		printHelp()
		return
	}

	if len(sCmdFlagDC) > 1 {
		fmt.Println("Creating default configuration file...")
		gpcconfig.WriteDefaultConfigFile(sCmdFlagDC)
		return
	}

	// READ CONFIG FILE
	tConfigData := gpcconfig.ReadConfigFromFile(sCmdFlagCF)

	// SETUP LOGGER
	gpclogging.Init(tConfigData.Logging.LogsFolder, // specify the directory to save the logfiles
		2,                                   // maximum logfiles allowed under the specified log directory
		1,                                   // number of logfiles to delete when number of logfiles exceeds the configured limit
		tConfigData.Logging.LogFileSizeMB,   // maximum size of a logfile in MB
		tConfigData.Logging.LogDebugEnabled) // whether logs with Debug level are written down
	gpclogging.Info("Application sucessfully initalized. Starting up")

	// LETS DO THE ACTUAL WORK
	gpcprocessmgr.StartProcessesFromConfig(&tConfigData, &shutdownWaitGroup)

	// GO TO SLEEP HERE IN MAIN AND WAIT FOR A SHUTDOWN REQUEST
	<-appEnd
	gpclogging.Info("Application shutting down...")
	shutdownWaitGroup.Wait()
}
