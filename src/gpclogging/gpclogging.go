package gpclogging

// Adopted from the original source at: https://github.com/antigloss
// Tributes to antigloss !!
// I modified it slightly, so that it writes to a single file only

/*
Package gpclogging is a logging facility which provides functions Debug, Info, Warn, Error, Panic and Abort to
write logs with different severity levels.

Features:

	1. Auto rotation: It'll create a new logfile whenever day changes or size of the current logfile exceeds the configured size limit.
	2. Auto purging: It'll delete some oldest logfiles whenever the number of logfiles exceeds the configured limit.
	3. Logs are not buffered, they are written to logfiles immediately with os.(*File).Write().
	4. Symlinks `PROG_NAME`.`USER_NAME`.`SEVERITY_LEVEL` will always link to the most current logfiles.
	5. Goroutine-safe.

*/
import (
	"fmt"
	"os"
	"path"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// consts
const (
	maxInt64          = int64(^uint64(0) >> 1)
	logCreatedTimeLen = 24
	logFilenameMinLen = 29
)

// log level
const (
	logLevelDebug = iota
	logLevelInfo
	logLevelWarn
	logLevelError
	logLevelMax
)

// log flags
const (
	logFlagLogDebug = 1 << iota
	logFlagLogFuncName
	logFlagLogFilenameLineNum
	logFlagLogToConsole
)

// const strings
const (
	// Default filename prefix for logfiles
	//DefFilenamePrefix = "%P.%H.%U"
	DefFilenamePrefix = "%P"

	logLevelChar = "DIWE"
)

// logger
type logger struct {
	file  *os.File
	level int
	day   int
	size  int64
	lock  sync.Mutex
}

// ###########################################################
// GLOBALS
// ###########################################################
var gProgname = path.Base(os.Args[0])

var gLogLevelNames = [logLevelMax]string{
	"DEBUG", "INFO", "WARN", "ERROR",
}

var gConf = config{
	logPath:     "./log/",
	logflags:    logFlagLogFilenameLineNum,
	maxfiles:    400,
	nfilesToDel: 10,
	maxsize:     100 * 1024 * 1024,
}

var gBufPool bufferPool
var gLogger logger

// Init must be called first, otherwise this logger will not function properly!
// It returns nil if all goes well, otherwise it returns the corresponding error.
//   maxfiles: Must be greater than 0 and less than or equal to 100000.
//   nfilesToDel: Number of files deleted when number of log files reaches `maxfiles`.
//                Must be greater than 0 and less than or equal to `maxfiles`.
//   maxsize: Maximum size of a log file in MB, 0 means unlimited.
//   logDebug: If set to false, `logger.Debug("xxxx")` will be mute.
func Init(logpath string, maxfiles, nfilesToDel int, maxsize uint32, logDebug bool) error {
	err := os.MkdirAll(logpath, 0755)
	if err != nil {
		return err
	}

	if maxfiles <= 0 || maxfiles > 100000 {
		return fmt.Errorf("maxfiles must be greater than 0 and less than or equal to 100000: %d", maxfiles)
	}

	if nfilesToDel <= 0 || nfilesToDel > maxfiles {
		return fmt.Errorf("nfilesToDel must be greater than 0 and less than or equal to maxfiles! toDel=%d maxfiles=%d",
			nfilesToDel, maxfiles)
	}

	gConf.logPath = logpath + "/"
	gConf.setFlags(logFlagLogDebug, logDebug)
	gConf.maxfiles = maxfiles
	gConf.nfilesToDel = nfilesToDel
	gConf.setMaxSize(maxsize)
	return SetFilenamePrefix(DefFilenamePrefix)
}

// SetLogFunctionName sets whether to log down the function name where the log takes place.
// By default, function name is not logged down for better performance.
func SetLogFunctionName(on bool) {
	gConf.setFlags(logFlagLogFuncName, on)
}

// SetLogFilenameLineNum sets whether to log down the filename and line number where the log takes place.
// By default, filename and line number are logged down. You can turn it off for better performance.
func SetLogFilenameLineNum(on bool) {
	gConf.setFlags(logFlagLogFilenameLineNum, on)
}

// SetLogToConsole sets whether to output logs to the console.
// By default, logs are not output to the console.
func SetLogToConsole(on bool) {
	gConf.setFlags(logFlagLogToConsole, on)
}

// SetFilenamePrefix sets filename prefix for the logfiles.
//
// Filename format for logfiles is `PREFIX`.`SEVERITY_LEVEL`.`DATE_TIME`.log
// 3 kinds of placeholders can be used in the prefix: %P, %H and %U.
// %P means program name, %H means hostname, %U means username.
// The default prefix for a log filename is logger.DefFilenamePrefix ("%P.%H.%U").
func SetFilenamePrefix(logfilenamePrefix string) error {
	gConf.setFilenamePrefix(logfilenamePrefix)

	files, err := getLogfilenames(gConf.logPath)
	if err == nil {
		gConf.curfiles = len(files)
	}
	return err
}

// Debug logs down a log with Debug level.
// If parameter logDebug of logger.Init() is set to be false, no Debug logs will be logged down.
func Debug(format string, args ...interface{}) {
	if gConf.logDebug() {
		log(logLevelDebug, format, args)
	}
}

// Info logs down a log with info level.
func Info(format string, args ...interface{}) {
	log(logLevelInfo, format, args)
}

// Warn logs down a log with warning level.
func Warn(format string, args ...interface{}) {
	log(logLevelWarn, format, args)
}

// Error logs down a log with error level.
func Error(format string, args ...interface{}) {
	log(logLevelError, format, args)
}

// logger configuration
type config struct {
	logPath     string
	pathPrefix  string
	logflags    uint32
	maxfiles    int   // limit the number of log files under `logPath`
	curfiles    int   // number of files under `logPath` currently
	nfilesToDel int   // number of files deleted when reaching the limit of the number of log files
	maxsize     int64 // limit size of a log file
	purgeLock   sync.Mutex
}

func (conf *config) setFlags(flag uint32, on bool) {
	if on {
		conf.logflags = conf.logflags | flag
	} else {
		conf.logflags = conf.logflags & ^flag
	}
}

func (conf *config) logDebug() bool {
	return (conf.logflags & logFlagLogDebug) != 0
}

func (conf *config) logFuncName() bool {
	return (conf.logflags & logFlagLogFuncName) != 0
}

func (conf *config) logFilenameLineNum() bool {
	return (conf.logflags & logFlagLogFilenameLineNum) != 0
}

func (conf *config) logToConsole() bool {
	return (conf.logflags & logFlagLogToConsole) != 0
}

func (conf *config) setMaxSize(maxsize uint32) {
	if maxsize > 0 {
		conf.maxsize = int64(maxsize) * 1024 * 1024
	} else {
		conf.maxsize = maxInt64 - (1024 * 1024 * 1024 * 1024 * 1024)
	}
}

func (conf *config) setFilenamePrefix(filenamePrefix string) {
	/*
		host, err := os.Hostname()
		if err != nil {
			host = "Unknown"
		}

		username := "Unknown"
		curUser, err := user.Current()
		if err == nil {
			tmpUsername := strings.Split(curUser.Username, "\\") // for compatible with Windows
			username = tmpUsername[len(tmpUsername)-1]
		}
	*/
	conf.pathPrefix = conf.logPath
	if len(filenamePrefix) > 0 {
		filenamePrefix = strings.Replace(filenamePrefix, "%P", gProgname, -1)
		//		filenamePrefix = strings.Replace(filenamePrefix, "%H", host, -1)
		//		filenamePrefix = strings.Replace(filenamePrefix, "%U", username, -1)
		conf.pathPrefix = conf.pathPrefix + filenamePrefix + "."
	}
}

func (l *logger) log(t time.Time, data []byte) {
	y, m, d := t.Date()

	l.lock.Lock()
	defer l.lock.Unlock()
	if l.size >= gConf.maxsize || l.day != d || l.file == nil {
		hour, min, sec := t.Clock()

		gConf.purgeLock.Lock()
		hasLocked := true
		defer func() {
			if hasLocked {
				gConf.purgeLock.Unlock()
			}
		}()
		// reaches limit of number of log files
		if gConf.curfiles >= gConf.maxfiles {
			files, err := getLogfilenames(gConf.logPath)
			if err != nil {
				l.errlog(t, data, err)
				return
			}

			gConf.curfiles = len(files)
			if gConf.curfiles >= gConf.maxfiles {
				sort.Sort(byCreatedTime(files))
				nfiles := gConf.curfiles - gConf.maxfiles + gConf.nfilesToDel
				if nfiles > gConf.curfiles {
					nfiles = gConf.curfiles
				}
				for i := 0; i < nfiles; i++ {
					err := os.RemoveAll(gConf.logPath + files[i])
					if err == nil {
						gConf.curfiles--
					} else {
						l.errlog(t, nil, err)
					}
				}
			}
		}

		//filename := fmt.Sprintf("%s%s.%d%02d%02d%02d%02d%02d%06d.log", gConf.pathPrefix, gLogLevelNames[l.level],
		//	y, m, d, hour, min, sec, (t.Nanosecond() / 1000))

		filename := fmt.Sprintf("%s%d%02d%02d%02d%02d%02d.log", gConf.pathPrefix, y, m, d, hour, min, sec)

		newfile, err := os.OpenFile(filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			l.errlog(t, data, err)
			return
		}
		gConf.curfiles++
		gConf.purgeLock.Unlock()
		hasLocked = false

		l.file.Close()
		l.file = newfile
		l.day = d
		l.size = 0
	}
	n, _ := l.file.Write(data)
	l.size += int64(n)
}

// (l *logger).errlog() should only be used within (l *logger).log()
func (l *logger) errlog(t time.Time, originLog []byte, err error) {
	buf := gBufPool.getBuffer()

	genLogPrefix(buf, l.level, 2, t)
	buf.WriteString(err.Error())
	buf.WriteByte('\n')
	if l.file != nil {
		l.file.Write(buf.Bytes())
		if len(originLog) > 0 {
			l.file.Write(originLog)
		}
	} else {
		fmt.Fprint(os.Stderr, buf.String())
		if len(originLog) > 0 {
			fmt.Fprint(os.Stderr, string(originLog))
		}
	}

	gBufPool.putBuffer(buf)
}

// sort files by created time embedded in the filename
type byCreatedTime []string

func (a byCreatedTime) Len() int {
	return len(a)
}

func (a byCreatedTime) Less(i, j int) bool {
	s1, s2 := a[i], a[j]
	if len(s1) < logFilenameMinLen {
		return true
	} else if len(s2) < logFilenameMinLen {
		return false
	} else {
		return s1[len(s1)-logCreatedTimeLen:] < s2[len(s2)-logCreatedTimeLen:]
	}
}

func (a byCreatedTime) Swap(i, j int) {
	a[i], a[j] = a[j], a[i]
}

// init is called after all the variable declarations in the package have evaluated their initializers,
// and those are evaluated only after all the imported packages have been initialized.
// Besides initializations that cannot be expressed as declarations, a common use of init functions is to verify
// or repair correctness of the program state before real execution begins.
func init() {
	tmpProgname := strings.Split(gProgname, "\\") // for compatible with `go run` under Windows
	gProgname = tmpProgname[len(tmpProgname)-1]

	gConf.setFilenamePrefix(DefFilenamePrefix)
}

// helpers
func getLogfilenames(dir string) ([]string, error) {
	var filenames []string
	f, err := os.Open(dir)
	if err == nil {
		filenames, err = f.Readdirnames(0)
		f.Close()
		if err == nil {

		}
	}
	return filenames, err
}

func genLogPrefix(buf *buffer, logLevel, skip int, t time.Time) {
	h, m, s := t.Clock()

	// time
	buf.tmp[0] = logLevelChar[logLevel]
	buf.tmp[1] = '-'
	buf.twoDigits(2, h)
	buf.tmp[4] = ':'
	buf.twoDigits(5, m)
	buf.tmp[7] = ':'
	buf.twoDigits(8, s)
	buf.Write(buf.tmp[:10])

	var pc uintptr
	var ok bool
	if gConf.logFilenameLineNum() {
		var file string
		var line int
		pc, file, line, ok = runtime.Caller(skip)
		if ok {
			buf.WriteByte(' ')
			buf.WriteString(path.Base(file))
			buf.tmp[0] = ':'
			n := buf.someDigits(1, line)
			buf.Write(buf.tmp[:n+1])
		}
	}
	if gConf.logFuncName() {
		if !ok {
			pc, _, _, ok = runtime.Caller(skip)
		}
		if ok {
			buf.WriteByte(' ')
			buf.WriteString(runtime.FuncForPC(pc).Name())
		}
	}

	buf.WriteString("] ")
}

func log(logLevel int, format string, args []interface{}) {
	buf := gBufPool.getBuffer()

	t := time.Now()
	genLogPrefix(buf, logLevel, 3, t)
	fmt.Fprintf(buf, format, args...)
	buf.WriteByte('\n')
	output := buf.Bytes()

	gLogger.log(t, output)

	if gConf.logToConsole() {
		fmt.Print(string(output))
	}

	gBufPool.putBuffer(buf)
}

// GetLogFileForProcess provides a opened file for logging process output
func GetLogFileForProcess(execName string) (*os.File, error) {

	t := time.Now()
	y, m, d := t.Date()
	hour, min, sec := t.Clock()

	outFileName := fmt.Sprintf("%s/%s_%d%02d%02d%02d%02d%02d.log", gConf.logPath, execName, y, m, d, hour, min, sec)
	return os.Create(outFileName)
}
