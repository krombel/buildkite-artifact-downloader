package fdroidHandler

import (
	"fmt"
	"os"
	"os/exec"

	log "github.com/sirupsen/logrus"
)

type FdroidHandler struct {
	virtualEnv string
}

func NewFdroidHandler() *FdroidHandler {
	return &FdroidHandler{
		virtualEnv: "",
	}
}

func stringIsDirectory(name string) (bool, error) {
	file, err := os.Open(name)
	if err != nil {
		return false, err
	}
	defer file.Close()
	fi, err := file.Stat()
	if err != nil {
		return false, err
	}
	if fi.IsDir() {
		return true, nil
	}
	return false, nil
}

func (fh *FdroidHandler) SetFdroidVENV(venv string) error {
	log.WithFields(log.Fields{
		"method": "SetFdroidVENV",
		"param":  venv,
	}).Info("Run")
	if ret, err := stringIsDirectory(venv); !ret {
		return fmt.Errorf("VENV is no directory (%v)", err)
	}
	if ret, err := stringIsDirectory(venv + "/bin"); !ret {
		return fmt.Errorf("VENV/bin is no directory (%v)", err)
	}
	fh.virtualEnv = venv
	// we set it here as

	log.WithFields(log.Fields{
		"method": "SetFdroidVENV",
		"param":  venv,
	}).Info("Done")
	return nil
}

// RunFdroidCommand executes "fdroid <command>" while setting venv if setup
func (fh *FdroidHandler) RunFdroidCommand(fdroidCommand string) {
	//cmd := exec.Command("fdroid", fdroidCommand)
	var backupPath string
	if fh.virtualEnv != "" {
		backupPath := os.Getenv("PATH")
		log.WithFields(log.Fields{
			"path":       backupPath,
			"virtualenv": fh.virtualEnv,
		}).Info("Set virtualenv for execution")
		os.Setenv("PATH", fh.virtualEnv+`/bin:`+backupPath)
	}

	cmd := exec.Command("fdroid", fdroidCommand)
	if fh.virtualEnv != "" {
		cmd.Env = append(os.Environ(),
			`VIRTUAL_ENV=`+fh.virtualEnv,
		)
	}

	cmd.Stdout = log.WithFields(log.Fields{
		"cmd": "fdroid",
	}).Writer()
	cmd.Stderr = log.WithFields(log.Fields{
		"cmd": "fdroid",
	}).WriterLevel(log.WarnLevel)

	log.WithFields(log.Fields{
		"virtualenv": fh.virtualEnv,
	}).Info("Runs fdroid " + fdroidCommand)
	if err := cmd.Run(); err != nil {
		log.Fatal(err)
	}

	if backupPath != "" {
		os.Setenv("PATH", backupPath)
	}
}
