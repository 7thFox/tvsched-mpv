package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/DexterLB/mpvipc"
)

type mpv_cmd struct {
	Command   []any `json:"command"`
	RequestId int64 `json:"request_id"`
}

var configPath = flag.String("config", "./config.json", "path to config file")

func main() {
	var err error

	flag.Parse()
	absPath, err := filepath.Abs(*configPath)
	if err != nil {
		fatalf("Failed to resolve relative path '%s': %s", *configPath, err.Error())
	}
	*configPath = absPath
	debugf("Config File: %s", *configPath)
	config, err := loadConfig(*configPath)
	if err != nil {
		fatalf("Failed to load config: %s", err.Error())
	}

	rand.Seed(time.Now().Unix())
	socketName := fmt.Sprintf("/tmp/tvsched-mpv-%d.sock", rand.Int31())
	// socketName := "/tmp/tvsched-mpv.sock"
	debugf("Socket: %s", socketName)

	debugf("deleting any existing socket file")
	err = os.Remove(socketName)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		errorf("Failed to delete existing socket: %s", err.Error())
		return
	}

	mpvEvents := make(chan *mpvipc.Event, 256)
	mpvExited := make(chan struct{})

	go func() {
		debugf("starting mpv")
		mpv := exec.Command("mpv",
			"--idle",
			fmt.Sprintf("--input-ipc-server=%s", socketName))
		err := mpv.Run()
		debugf("mpv exited")
		mpvExited <- struct{}{}
		if err != nil {
			errorf("mpv exited with error: %s", err.Error())
		}
	}()

	debugf("waiting for mpv to open socket")
	for {
		_, stat := os.Stat(socketName)
		if !errors.Is(stat, os.ErrNotExist) {
			break
		}
	}

	debugf("connecting to mpv")
	conn := mpvipc.NewConnection(socketName)
	err = conn.Open()
	if err != nil {
		errorf(err.Error())
		return
	}

	defer conn.Close()
	debugf("connected to mpv")

	go func() {
		idleStarted := false
		for e := range mpvEvents {
			tracef("Event: %s", e.Name)

			switch e.Name {
			case "end-file":
				debugf("Ended playback")
				fallthrough
			case "idle":
				if !idleStarted && !conn.IsClosed() {
					idleStarted = true
					_, err := conn.Call("loadfile", config.next_video())
					if err != nil {
						errorf("Error playing next video: %s", err.Error())
					}
				}
			case "start-file":
				idleStarted = false
			case "audio-reconfig":
			case "file-loaded":
			case "video-reconfig":
			case "playback-restart":
			case "seek":
				break // explictly ignored
			default:
				warnf("Unhandeled event: %s", e.Name)
			}
		}
	}()

	conn.ListenForEvents(mpvEvents, mpvExited)
	debugf("exiting")
}

// func startNext(conn *mpvipc.Connection, queue *video_queue) {
// 	path := queue.NextVideo()
// 	debugf("Playing next video: %s", path)
// }

func tracef(msg string, params ...any) {
	// fmt.Printf("[TRACE] "+msg+"\n", params...)
}
func debugf(msg string, params ...any) {
	fmt.Printf("\033[35m[DEBUG] "+msg+"\033[37m\n", params...)
}
func infof(msg string, params ...any) {
	fmt.Printf("\033[34m[INFO]  "+msg+"\033[37m\n", params...)
}
func warnf(msg string, params ...any) {
	fmt.Printf("\033[33m[WARN]  "+msg+"\033[37m\n", params...)
}
func errorf(msg string, params ...any) {
	fmt.Printf("\033[31m[ERROR] "+msg+"\033[37m\n", params...)
}
func fatalf(msg string, params ...any) {
	errorf(msg, params...)
	os.Exit(2)
}
