// picture: a picture of yours on the robot's screen (or four numbered quarters), then the quarter
// the person taps. Dev fits the picture to the screen; up to about 1.5 MB.
//
//	go run ./examples/go/picture [photo.jpg]
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"

	"github.com/i41-eu/q41eu-dev/client"
	"github.com/i41-eu/q41eu-dev/examples"
)

func main() {
	pic, err := fs.ReadFile(examples.FS, "img/quarters.png")
	if len(os.Args) > 1 {
		pic, err = os.ReadFile(os.Args[1])
	}
	if err != nil {
		log.Fatal(err)
	}
	robot, err := client.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	var quarters []client.Region
	for i := range 4 {
		quarters = append(quarters, client.Region{ID: fmt.Sprint(i + 1), Rect: []int{160 * (i % 2), 120 * (i / 2), 160, 120}})
	}
	tap, err := robot.Clickmap(context.Background(), pic, quarters...)
	if err != nil {
		log.Fatal(err)
	}
	if tap.Region == "" {
		fmt.Println("no tap within 30 s")
		os.Exit(1)
	}
	fmt.Printf("quarter %s (at %d, %d)\n", tap.Region, tap.X, tap.Y)
}
