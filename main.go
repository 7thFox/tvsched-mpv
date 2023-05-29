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

var configPath = flag.String("config", "./config.jsonc", "path to config file")
var useSock = flag.String("sock", "", "existing socket to use -- will create a new socket and launch mpv if not provided -- launch mpv with \"mpv --idle --input-ipc-server=SOCKET_NAME\"")

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

	mpvExited := make(chan struct{})

	var socketName string
	if *useSock == "" {
		socketName = fmt.Sprintf("/tmp/tvsched-mpv-%d.sock", rand.Int31())
		// socketName := "/tmp/tvsched-mpv.sock"
		debugf("Socket: %s", socketName)

		debugf("Deleting any existing socket file")
		err = os.Remove(socketName)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			errorf("Failed to delete existing socket: %s", err.Error())
			return
		}

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

		debugf("Waiting for mpv to open socket")
		for {
			_, stat := os.Stat(socketName)
			if !errors.Is(stat, os.ErrNotExist) {
				break
			}
		}
	} else {
		socketName = *useSock
		_, stat := os.Stat(socketName)
		if errors.Is(stat, os.ErrNotExist) {
			fatalf("Socket provided does not exist")
		}
	}

	mpvEvents := make(chan *mpvipc.Event, 256)

	debugf("Connecting to mpv")
	conn := mpvipc.NewConnection(socketName)
	err = conn.Open()
	if err != nil {
		errorf(err.Error())
		return
	}

	defer conn.Close()
	debugf("Connected to mpv")

	go func() {
		idleStarted := false

		playback_next(conn, config)

		for e := range mpvEvents {
			tracef("Event: %s", e.Name)

			switch e.Name {
			case "end-file":
				debugf("Ended playback")
				fallthrough
			case "idle":
				if !idleStarted {
					idleStarted = true
					playback_next(conn, config)
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

func playback_next(conn *mpvipc.Connection, config *sched_config) {
	if !conn.IsClosed() {
		next := config.next_video()
		debugf("Starting playback: %s", next)
		if config.HistoryFilePath != "" {
			his, err := os.OpenFile(config.HistoryFilePath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0664)
			if err != nil {
				errorf("Failed to open history file: %s", err.Error())
			} else {
				fmt.Fprintf(his, "%s\t%s", time.Now().Format(time.DateTime), next)
				if err = his.Close(); err != nil {
					errorf("Failed to close history file: %s", err.Error())
				}
			}
		}
		_, err := conn.Call("loadfile", next)
		if err != nil {
			errorf("Error playing next video: %s", err.Error())
		}
	}

}

// func startNext(conn *mpvipc.Connection, queue *video_queue) {
// 	path := queue.NextVideo()
// 	debugf("Playing next video: %s", path)
// }

func tracef(msg string, params ...any) {
	// fmt.Printf("[TRACE] "+msg+"\n", params...)
}
func debugf(msg string, params ...any) {
	fmt.Printf("%s ", time.Now().Format(time.DateTime))
	fmt.Printf("\033[35m[DEBUG] "+msg+"\033[37m\n", params...)
}
func infof(msg string, params ...any) {
	fmt.Printf("%s ", time.Now().Format(time.DateTime))
	fmt.Printf("\033[34m[INFO]  "+msg+"\033[37m\n", params...)
}
func warnf(msg string, params ...any) {
	fmt.Printf("%s ", time.Now().Format(time.DateTime))
	fmt.Printf("\033[33m[WARN]  "+msg+"\033[37m\n", params...)
}
func errorf(msg string, params ...any) {
	fmt.Printf("%s ", time.Now().Format(time.DateTime))
	fmt.Printf("\033[31m[ERROR] "+msg+"\033[37m\n", params...)
}
func fatalf(msg string, params ...any) {
	errorf(msg, params...)
	os.Exit(2)
}
