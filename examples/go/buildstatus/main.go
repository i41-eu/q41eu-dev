// buildstatus runs a command and shows on the robot how it went: blue while it runs, then a
// green card and a chime when it passed, a red card and a shake when it failed. Its exit code is
// the command's.
//
//	go run ./examples/go/buildstatus go test ./...
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/i41-eu/q41eu-dev/client"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"go", "test", "./..."}
	}
	ctx := context.Background()
	robot, err := client.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	name := strings.Join(args, " ")
	robot.Show(ctx, client.Card{Title: "Running", Text: name, Color: "blue"})
	robot.Do(ctx, "leds", map[string]any{"effect": "breathe", "color": "blue"}, nil)

	start := time.Now()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	runErr := cmd.Run()
	took := fmt.Sprintf("%s · %.0f s", name, time.Since(start).Seconds())

	if runErr == nil {
		robot.Show(ctx, client.Card{Title: "Passed", Text: "All good", Note: took, Color: "green", Progress: client.Progress(1)})
		robot.LEDs(ctx, "green")
		robot.Chime(ctx, "tada")
		robot.Gesture(ctx, "nod")
		return
	}
	robot.Show(ctx, client.Card{Title: "Failed", Text: "Have a look", Note: took, Color: "red"})
	robot.LEDs(ctx, "red")
	robot.Chime(ctx, "error")
	robot.Gesture(ctx, "shake")
	if exit, ok := runErr.(*exec.ExitError); ok {
		os.Exit(exit.ExitCode())
	}
	log.Fatal(runErr)
}
