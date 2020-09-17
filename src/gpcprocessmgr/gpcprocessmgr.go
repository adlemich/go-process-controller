package gpcprocessmgr

import (
	"context"
	"fmt"
	"gpcconfig"
	"gpclogging"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
)

//#######################################################
//### GLOBAL VARIABLES, INIT, CONSTS
//#######################################################

var gProcRuntimeData map[string]*GPCProcRuntimeData
var gRuntimeDatatMux sync.Mutex
var gStopMon bool
var gStopMux sync.Mutex

/*ShutdownAll will stop the monitoring routine and will
then try to terminate all started processes if configured so
---------------------------------------------------------------------------------------*/
func ShutdownAll() {
	gpclogging.Debug("Entering ShutdownAll()")

	// Stop the monitoring routine
	gStopMux.Lock()
	gStopMon = true
	gStopMux.Unlock()

	// Terminate all started processes (gracefully)
	gRuntimeDatatMux.Lock()
	defer gRuntimeDatatMux.Unlock()

	for procName, runtimeData := range gProcRuntimeData {
		gpclogging.Debug("Start to shut donw all running processes...")

		//gpclogging.Debug("Checking process <%s>.", procName)
		err := checkProcessRunning(runtimeData.procStatus.pid)

		if err != nil {
			// Process has exited
			gpclogging.Debug("Process <%s>, PID=<%d> has exited. Nothing to do.", procName, runtimeData.procStatus.pid)
		} else {
			gpclogging.Info("Will now try to kill Process <%s>, PID=<%d>.", procName, runtimeData.procStatus.pid)
			// Process is still active - send termination signal
			errKill := killProcess(runtimeData.procCmd)
			if errKill != nil {
				gpclogging.Error("Process <%s>, PID=<%d> could not be killed!! <%s>", procName, runtimeData.procStatus.pid, errKill.Error())
			}
		}

		// Set flags and close log file
		runtimeData.procStatus.active = false
		if runtimeData.procLog != nil {
			runtimeData.procLog.Close()
		}
	}

	gpclogging.Debug("Leave ShutdownAll()")
}

//StartProcessesFromConfig reads the configuration and starts processes
//#########################################################
func StartProcessesFromConfig(configData *gpcconfig.ConfigData, shutdownWaitGroup *sync.WaitGroup) {
	gpclogging.Debug("Entering StartProcessesFromConfig(). Will now begin to launch processes.")

	// Build the inital data management set
	gRuntimeDatatMux.Lock()
	gProcRuntimeData = make(map[string]*GPCProcRuntimeData)
	for configIndex := range configData.Tasks {
		gpclogging.Debug("Building runtime config at index <%d>: ProcPath =<%s>.", configIndex, configData.Tasks[configIndex].Path)
		gProcRuntimeData[configData.Tasks[configIndex].Name] = NewProcRuntimeData(&configData.Tasks[configIndex])
		gpclogging.Debug("Config check for prog <%s>: ProcPath =<%s>.", gProcRuntimeData[configData.Tasks[configIndex].Name].procConfig.Name, gProcRuntimeData[configData.Tasks[configIndex].Name].procConfig.Path)
	}
	gRuntimeDatatMux.Unlock()

	// Start a goroutine that checks the running processes in background
	gStopMux.Lock()
	gStopMon = false
	gStopMux.Unlock()

	go func() {
		shutdownWaitGroup.Add(1)
		monitorProcesses(shutdownWaitGroup)
		shutdownWaitGroup.Done()
	}()

	for procName, runtimeData := range gProcRuntimeData {
		gpclogging.Debug("Working on inital start for <%s>. WaitForExitTimeout = <%d>", procName, runtimeData.procConfig.WaitForExitTimeoutS)
		// now start all configured processes, differentiate wait and nowait here
		if runtimeData.procConfig.WaitForExitTimeoutS > 0 {
			go func(procName string, runtimeData *GPCProcRuntimeData, shutdownWaitGroup *sync.WaitGroup) {
				// Pause here until Start delay is reached
				gpclogging.Debug("Process <%s> is configured with start delay <%d>s. Will now wait if configured so.",
					procName, runtimeData.procConfig.StartDelayS)
				// TODO - Change to a select statement that waits for termination or timeout
				time.Sleep(time.Duration(runtimeData.procConfig.StartDelayS) * time.Second)
				// Now go ahead
				gpclogging.Debug("Launching wait process...")
				shutdownWaitGroup.Add(1)
				launchProcessAndWait(procName)
				shutdownWaitGroup.Done()
			}(procName, runtimeData, shutdownWaitGroup)

		} else {
			go func(procName string, runtimeData *GPCProcRuntimeData, shutdownWaitGroup *sync.WaitGroup) {
				// Pause here until Start delay is reached
				gpclogging.Debug("Process <%s> is configured with start delay <%d>s. Will now wait if configured so.",
					procName, runtimeData.procConfig.StartDelayS)
				// TODO - Change to a select statement that waits for termination or timeout
				time.Sleep(time.Duration(runtimeData.procConfig.StartDelayS) * time.Second)
				// Now go ahead
				gpclogging.Debug("Launching no-wait process...")
				shutdownWaitGroup.Add(1)
				launchProcess(procName)
				shutdownWaitGroup.Done()
			}(procName, runtimeData, shutdownWaitGroup)
		}
	}
	gpclogging.Debug("Leaving StartProcessesFromConfig()")
}

//monitorProcesses checks the status of each process every 100 ms
//#########################################################
func monitorProcesses(shutdownWaitGroup *sync.WaitGroup) {
	gpclogging.Debug("Entering monitorProcesses().")

	// run forever until application is closed
	for gStopMon == false {
		// gpclogging.Debug("Now checking process status.")

		for procName, runtimeData := range gProcRuntimeData {
			// Do this only for active processes that were started with No-Wait
			if runtimeData.procConfig.WaitForExitTimeoutS < 1 &&
				runtimeData.procCmd != nil &&
				runtimeData.procStatus.active {

				// Check if the process is still running
				//gpclogging.Debug("Checking process <%s>.", procName)
				err := checkProcessRunning(runtimeData.procStatus.pid)

				if err != nil {
					// Process has exited
					gpclogging.Warn("Process <%s>, PID=<%d> has exited.", procName,
						runtimeData.procStatus.pid)

					// Set flags and close log file
					runtimeData.procStatus.active = false
					if runtimeData.procLog != nil {
						runtimeData.procLog.Close()
					}

					// Now should check if the process shall be automatically restarted
					if runtimeData.procConfig.MaxRestarts > 0 {
						if runtimeData.procStatus.restartCount < runtimeData.procConfig.MaxRestarts {
							go func(procName string, runtimeData *GPCProcRuntimeData, shutdownWaitGroup *sync.WaitGroup) {
								runtimeData.procStatus.restartCount++
								gpclogging.Info("Will now try to restart no-wait process <%s>. This is attempt No <%d>..", procName, runtimeData.procStatus.restartCount)
								shutdownWaitGroup.Add(1)
								launchProcess(procName)
								shutdownWaitGroup.Done()
							}(procName, runtimeData, shutdownWaitGroup)
						} else {
							gpclogging.Error("Process <%s> has reached the max restart count of <%d>. WILL NOT RESTART THE PROCESS.",
								procName, runtimeData.procConfig.MaxRestarts)
							runtimeData.procStatus.error = true
						}
					}
				}
			}
		}

		// Sleep for 100ms
		// TODO - Change to a select statement that waits for termination or timeout
		time.Sleep(100 * time.Millisecond)
	}
	gpclogging.Debug("Leaving monitorProcesses().")
}

//checkProcessRunning checks if a program for a given PID is still active
//this a manual fix for https://github.com/golang/go/issues/33814
//#########################################################
func checkProcessRunning(pid int) (err error) {
	const da = syscall.STANDARD_RIGHTS_READ | syscall.PROCESS_QUERY_INFORMATION | syscall.SYNCHRONIZE
	h, e := syscall.OpenProcess(da, true, uint32(pid))
	defer syscall.CloseHandle(h)

	if e != nil {
		return os.NewSyscallError("OpenProcess", e)
	}
	return nil
}

//launchProcess launches a process, no waiting here
//#########################################################
func launchProcess(procName string) {
	gpclogging.Debug("Entering launchProcess()")

	// Lock configuration until function ended
	gRuntimeDatatMux.Lock()
	gRuntimeDatatMux.Unlock()

	gpclogging.Info("Will now try to launch process <%s>.", procName)

	// Start process - fire and forget
	gProcRuntimeData[procName].procCmd = exec.Command(gProcRuntimeData[procName].procConfig.Path)
	doProcessSettings(gProcRuntimeData[procName])

	err := gProcRuntimeData[procName].procCmd.Start()

	if err != nil {
		gpclogging.Error("Could not start process <%s>, Error message is <%s>", procName, err)
		gProcRuntimeData[procName].procStatus.error = true
	} else {
		gpclogging.Info("Starting process <%s> OK!", procName)
		gProcRuntimeData[procName].procStatus.pid = gProcRuntimeData[procName].procCmd.Process.Pid
		gProcRuntimeData[procName].procStatus.active = true
	}
	gProcRuntimeData[procName].procCmd.Process.Release()

	gpclogging.Debug("Leaving launchProcess()")
}

//launchProcessAndWait launches a process and waits for it to complete
//########################################################################
func launchProcessAndWait(procName string) {
	gpclogging.Debug("Entering launchProcess()")

	// Lock configuration until function ended
	gRuntimeDatatMux.Lock()
	defer gRuntimeDatatMux.Unlock()

	gpclogging.Info("Will now try to launch process <%s> with wait option, timeout is <%d>s.", procName, gProcRuntimeData[procName].procConfig.WaitForExitTimeoutS)

	// Run process and wait for a max amount of time for exit
	sDurationString := fmt.Sprintf("%ds", gProcRuntimeData[procName].procConfig.WaitForExitTimeoutS)
	timeoutDur, parseErr := time.ParseDuration(sDurationString)
	if parseErr != nil {
		gpclogging.Error("Could not parse execution wait timeout config <%s>, Error message is <%d>", sDurationString, parseErr.Error())
		gProcRuntimeData[procName].procStatus.error = true
		return
	}

	progContext, cancel := context.WithTimeout(context.Background(), timeoutDur)
	defer cancel()

	gProcRuntimeData[procName].procCmd = exec.CommandContext(progContext, gProcRuntimeData[procName].procConfig.Path)
	doProcessSettings(gProcRuntimeData[procName])

	err := gProcRuntimeData[procName].procCmd.Run()
	gProcRuntimeData[procName].procStatus.active = true

	if err != nil {
		switch err.(type) {
		default:
			// STARTUP ERROR
			gpclogging.Error("Could not run process <%s>, Error message is: %s", gProcRuntimeData[procName].procConfig.Path, err.Error())
			gProcRuntimeData[procName].procStatus.error = true
		case *exec.ExitError:
			// TIMEOUT
			gpclogging.Warn("Running process <%s> OK but it was termined after configured timeout! Exit code was <%d>",
				gProcRuntimeData[procName].procConfig.Path, gProcRuntimeData[procName].procCmd.ProcessState.ExitCode())
			gProcRuntimeData[procName].procStatus.timeout = true
			gProcRuntimeData[procName].procStatus.done = true
		}
	} else {
		gpclogging.Info("Running process <%s> OK! Exit code was <%d>", gProcRuntimeData[procName].procConfig.Path,
			gProcRuntimeData[procName].procCmd.ProcessState.ExitCode())
		gProcRuntimeData[procName].procStatus.done = true
	}

	gpclogging.Debug("Leaving launchProcessAndWait()")
}

// doProcessSettings will tweak the Cmd structure with specific runtime settings
//------------------------------------------------------------------------------
func doProcessSettings(proc *GPCProcRuntimeData) {
	gpclogging.Debug("Entering doProcessSettings() for process <%s>", proc.procConfig.Name)

	sysProcSettings := syscall.SysProcAttr{}

	// Setting input, output and error streams
	proc.procCmd.Stdin = nil

	logOut, err := gpclogging.GetLogFileForProcess(proc.procConfig.Name)
	if err != nil {
		gpclogging.Error("Could not open log file for process <%s> with error <%s>", proc.procConfig.Name, err.Error())
	} else {
		outWriter := io.Writer(logOut)
		proc.procCmd.Stderr, proc.procCmd.Stdout = outWriter, outWriter

		// Store for later closing
		proc.procLog = logOut
	}

	// Hide the window
	if proc.procConfig.HideWindow {
		gpclogging.Debug("Process <%s>, HideWindow enabled, setting SysProcAttributes.", proc.procConfig.Name)
		sysProcSettings.HideWindow = true
	} else {
		gpclogging.Debug("Process <%s>, HideWindow disabled, setting SysProcAttributes.", proc.procConfig.Name)
		sysProcSettings.HideWindow = false
	}

	proc.procCmd.SysProcAttr = &sysProcSettings

	gpclogging.Debug("Leaving doProcessSettings()")
}

//killProcess will try to kill the given process (windows specfic)
//-------------------------------------------------------------------
func killProcess(proc *exec.Cmd) error {
	gpclogging.Debug("Enter KillProcess()")

	proc.Process.Release()
	proc.Process.Signal(syscall.SIGTERM)
	proc.Process.Signal(syscall.SIGKILL)

	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(proc.Process.Pid))
	err := kill.Run()

	gpclogging.Debug("Leaving KillProcess()")

	return err
}
