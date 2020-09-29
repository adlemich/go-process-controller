package gpcconfig

import (
	"encoding/json"
	"log"
	"os"
)

//ProcessConfig is the in-memory representation of the configuration file part of process
type ProcessConfig struct {
	Name                string   // Name for the process to run
	StartPath           string   // Exact path to executable
	StartArgs           []string // Arguments passed to the executable
	StartDelayS         uint32   // zero => no start delay
	MaxRestarts         uint32   // zero => do not automatically restart
	WaitForExitTimeoutS uint32   // zero => no waiting for application to end. If specified, the process will be terminated when it exeeds the timeout
	HideWindow          bool     // true hides the window, false will show it
	StopPath            string   // Exact path to executable
	StopArgs            []string // Arguments passed to the executable
}

//ConfigData is the in-memory representation of the configuration file
type ConfigData struct {
	Logging struct {
		LogsFolder      string // folder where to store logs
		LogFileSizeMB   uint32 // Max file size for log file in MB
		LogDebugEnabled bool   // Enables debug output
	}
	Tasks []ProcessConfig // The actual processes that shall be started
}

//ReadConfigFromFile loads a active configuration from a JSON
//#########################################################
func ReadConfigFromFile(sConfigFilePath string) (tConfigData ConfigData) {

	fConfigFile, err := os.Open(sConfigFilePath)
	if err != nil {
		log.Fatal("Can't open config file: ", err)
	}
	// Close File when this function returns
	defer fConfigFile.Close()

	jsonDecoder := json.NewDecoder(fConfigFile)
	tConfigData = ConfigData{}

	err = jsonDecoder.Decode(&tConfigData)
	if err != nil {
		log.Fatal("Can't decode config JSON, using defaults: ", err)
	}

	return tConfigData
}

//WriteDefaultConfigFile writes a default configuration file to disk
//#########################################################
func WriteDefaultConfigFile(sConfigFilePath string) {

	fOutFile, err := os.Create(sConfigFilePath)
	if err != nil {
		log.Fatal("Can not create new default configuration file.", err)
		return
	}
	// Close file when this function ends
	defer fOutFile.Close()

	jsonEncoder := json.NewEncoder(fOutFile)
	tDefaultConf := ConfigData{}

	// Setting default values
	tDefaultConf.Logging.LogsFolder = "./logs"
	tDefaultConf.Logging.LogFileSizeMB = 20
	tDefaultConf.Logging.LogDebugEnabled = true

	p1 := ProcessConfig{}
	p2 := ProcessConfig{}

	p1.Name = "Notepad"
	p1.StartPath = "notepad.exe"
	p1.StartArgs = []string{"notepad.exe", "myfile.txt"}
	p1.StartDelayS = 0
	p1.MaxRestarts = 3
	p1.WaitForExitTimeoutS = 0
	p1.HideWindow = false
	p1.StopPath = ""
	p1.StopArgs = []string{"", ""}

	p2.Name = "Paint"
	p2.StartPath = "mspaint.exe"
	p2.StartArgs = []string{"mspaint.exe", ""}
	p2.StartDelayS = 5
	p2.MaxRestarts = 0
	p2.WaitForExitTimeoutS = 0
	p2.HideWindow = true
	p2.StopPath = ""
	p2.StopArgs = []string{"", ""}

	tDefaultConf.Tasks = make([]ProcessConfig, 0)
	tDefaultConf.Tasks = append(tDefaultConf.Tasks, p1)
	tDefaultConf.Tasks = append(tDefaultConf.Tasks, p2)

	// Writing file
	jsonEncoder.SetIndent("", "    ")
	encodeErr := jsonEncoder.Encode(&tDefaultConf)
	if encodeErr != nil {
		log.Fatal("Can not write to new default configuration file.", encodeErr)
		return
	}

}
