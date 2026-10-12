// events prints what people do at the robot and answers some of it: a pat on the head makes it
// chirp and smile, a tag held near shows what the tag says. Ctrl+C ends it.
//
//	go run ./examples/go/events
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/i41-eu/q41eu-dev/client"
)

func main() {
	ctx := context.Background()
	robot, err := client.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	_, last, err := robot.Events(ctx, 0, 0) // start from now: skip the ones from before
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("touch the robot: tap its screen, pat its head, hold a tag near it…")
	for {
		events, seq, err := robot.Events(ctx, last, 30*time.Second)
		if err != nil {
			log.Println(err)
			time.Sleep(3 * time.Second)
			continue
		}
		last = seq
		for _, e := range events {
			fmt.Println(e.Name, e.Data)
			switch e.Name {
			case "pat", "pet", "stroke":
				robot.Face(ctx, "happy")
				robot.Chime(ctx, "chirp")
			case "tag":
				robot.Show(ctx, client.Card{Title: "A tag", Text: fmt.Sprint(e.Data["text"]), Note: fmt.Sprint(e.Data["uid"])})
			case "tap":
				robot.Clear(ctx)
			}
		}
	}
}
