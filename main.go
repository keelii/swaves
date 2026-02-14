package main

import (
	"log"
	"os"
	"os/exec"
	"swaves/internal/types"
	"time"
)

func watchParent() {
	ppid := os.Getppid()

	go func() {
		for {
			time.Sleep(time.Second)
			pid := os.Getppid()
			if pid != ppid {
				log.Println("parent process exited, exiting worker", pid, ppid)
				os.Exit(1)
			}
		}
	}()
}
func worker() {
	watchParent()

	swv := NewApp(types.AppConfig{
		SqliteFile: "data.sqlite",
		ListenAddr: ":3000",
		AppName:    "swaves",
	})
	swv.Listen()

	defer swv.Shutdown()
}

func launcher() {
	for {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "WORKER=1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		log.Println("[launcher] start worker")
		err := cmd.Start()
		if err != nil {
			log.Println("[launcher] failed to start worker:", err)
			time.Sleep(time.Second)
			continue
		}

		// 阻塞等待 worker 退出
		err = cmd.Wait()
		log.Println("[launcher] worker exited:", err)

		time.Sleep(200 * time.Millisecond)
	}
}

func main() {
	swv := NewApp(types.AppConfig{
		SqliteFile: "data.sqlite",
		BackupDir:  "backups",
		ListenAddr: ":3000",
		AppName:    "swaves",
	})
	swv.Listen()

	defer swv.Shutdown()
	//
	//if os.Getenv("WORKER") == "1" {
	//	worker()
	//} else {
	//	launcher()
	//}
}
