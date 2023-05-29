package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"net/http"
	"path"
	"sort"
	"strings"
)

type config_group struct {
	Name     string `json:"name"`
	Priority *int   `json:"priority"`

	Directory *struct {
		Path  string `json:"path"`
		Order string `json:"order"`
		// Recursive bool   `json:"recursive"`

		last_played_index int
	} `json:"dir"`

	// YouTubeLivestreams *struct {
	// 	Inturrupt  *bool    `json:"inturrupt"`
	// 	ChannelIDs []string `json:"channels"`
	// } `json:"yt-live"`

	// Paths    []string `json:"paths"`
	// lastPath int
}

type sched_config struct {
	HistoryFilePath string `json:"history_file"`

	Groups []*config_group `json:"configs"`
}

func loadConfig(path string) (*sched_config, error) {
	debugf("Loading config")
	c := sched_config{}
	f, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(f, &c)
	if err != nil {
		return nil, err
	}

	debugf("Validating config")
	mutuallyExculsive := func(opts ...bool) bool {
		hasOne := false
		for _, x := range opts {
			if x {
				if hasOne {
					return false
				}
				hasOne = x
			}
		}
		return hasOne
	}

	for _, g := range c.Groups {
		if g.Name == "" {
			g.Name = "<unnamed group>"
		}
		if g.Priority == nil {
			return nil, fmt.Errorf("Group %s: not given a priority", g.Name)
		}

		if !mutuallyExculsive(
			g.Directory != nil) {
			return nil, fmt.Errorf("Group %s: May only use on video selection method", g.Name)
		}

		if g.Directory != nil {
			g.Directory.Order = strings.ToLower(g.Directory.Order)
			g.Directory.last_played_index = -1

			if g.Directory.Order == "" {
				g.Directory.Order = "asc"
			}

			if !strings.HasPrefix(g.Directory.Order, "asc") &&
				!strings.HasPrefix(g.Directory.Order, "desc") &&
				!strings.HasPrefix(g.Directory.Order, "rand") {
				return nil, fmt.Errorf("Group %s: Invalid sord order", g.Name)
			}
		}
	}

	sort.SliceStable(c.Groups, func(i, j int) bool {
		if c.Groups[i].Priority == c.Groups[j].Priority {
			warnf("Groups both have priority=%i: %s, %s", *c.Groups[i].Priority, c.Groups[i].Name, c.Groups[j].Name)
		}
		return *c.Groups[i].Priority < *c.Groups[j].Priority
	})

	debugf("Config Loaded")
	return &c, nil
}

func (c *sched_config) next_video() string {
	for _, g := range c.Groups {
		var n string
		if g.Directory != nil {
			info, err := ioutil.ReadDir(g.Directory.Path)
			if err != nil {
				errorf("Error reading directory %s: %s", g.Directory.Path, err.Error())
				continue
			}
			if len(info) == 0 {
				warnf("No files in directory: %s", g.Directory.Path)
				continue
			}

			var next int
			if strings.HasPrefix(g.Directory.Order, "asc") {
				if g.Directory.last_played_index < 0 {
					next = 0
				} else {
					next = (g.Directory.last_played_index + 1) % len(info)
				}
			} else if strings.HasPrefix(g.Directory.Order, "desc") {
				if g.Directory.last_played_index < 0 {
					next = len(info) - 1
				} else {
					next = (g.Directory.last_played_index + len(info) - 1) % len(info) // -1 w/ modulos
				}
			} else if strings.HasPrefix(g.Directory.Order, "rand") {
				next = rand.Int() & len(info)
				for next == g.Directory.last_played_index { // Don't repeat even though it's technically random
					next = rand.Int() & len(info)
				}
			} else {
				errorf("Invalid order not caught in config parsing! (%s)", g.Directory.Order)
				continue
			}

			g.Directory.last_played_index = next
			return path.Join(g.Directory.Path, info[next].Name())
		}
		// if g.Paths != nil {
		// 	n = g.Paths[g.lastPath]
		// 	g.lastPath = (g.lastPath + 1) % len(g.Paths)
		// } else if g.YouTubeLivestreams != nil {
		// 	for _, c := range g.YouTubeLivestreams.ChannelIDs {
		// 		debugf("Checking channel %s for livestream", c)
		// 		liveUrl := fmt.Sprintf("https://www.youtube.com/%s/live", c)
		// 		if isLivestreaming(liveUrl) {
		// 			n = liveUrl
		// 			break
		// 		}
		// 	}
		// }

		if n != "" {
			debugf("Next Video: %s [from %s]", n, g.Name)
			return n
		}
	}
	errorf("No next video found")
	return ""
}

func isLivestreaming(liveUrl string) bool {

	resp, err := http.Get(liveUrl)
	if err != nil {
		errorf("Failed to check for livestream: %s", err.Error())
		return false
	}
	return containsLivestreamSential(resp.Body)
}

func containsLivestreamSential(body io.ReadCloser) bool {
	const s = 2048
	buffprev := make([]byte, 0)
	buff := make([]byte, s*2)
	n, err := body.Read(buff)
	buff = buff[:n]

	for {
		if err != nil {
			if !errors.Is(err, io.EOF) {
				errorf("Failed to check for livestream: %s", err.Error())
			}
			return false
		}

		str := string(append(buffprev, buff...))
		if strings.Contains(str, "\"liveBadgeRenderer\":{\"label\":{\"simpleText\":\"LIVE NOW\"}}") {
			return true
		}

		buffprev = buff
		buff = make([]byte, s)
		n, err = body.Read(buff)
		buff = buff[:n]
	}
}
