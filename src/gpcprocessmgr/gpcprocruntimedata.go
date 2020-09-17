package gpcprocessmgr

import (
	"gpcconfig"
	"os"
	"os/exec"
)

// GPCProcRuntimeData holds runtime data
type GPCProcRuntimeData struct {
	procConfig *gpcconfig.ProcessConfig
	procCmd    *exec.Cmd
	procLog    *os.File
	procStatus struct {
		pid          int
		active       bool
		error        bool
		timeout      bool
		done         bool
		restartCount uint32
	}
}

// NewProcRuntimeData returns a default struct
func NewProcRuntimeData(configData *gpcconfig.ProcessConfig) *GPCProcRuntimeData {
	var out GPCProcRuntimeData

	out.procCmd = nil
	out.procConfig = configData
	out.procLog = nil
	out.procStatus.pid = 0
	out.procStatus.active = false
	out.procStatus.error = false
	out.procStatus.timeout = false
	out.procStatus.done = false
	out.procStatus.restartCount = 0

	return &out
}
