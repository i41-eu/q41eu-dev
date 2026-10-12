// ask: a question on the robot's screen; the person at the robot taps an answer.
//
//	go run ./examples/go/ask "Coffee or tea?" Coffee Tea
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/i41-eu/q41eu-dev/client"
)

func main() {
	question, answers := "Coffee or tea?", []string{"Coffee", "Tea"}
	if len(os.Args) > 3 {
		question, answers = os.Args[1], os.Args[2:]
	}
	robot, err := client.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	answer, err := robot.Ask(context.Background(), question, answers...)
	if err != nil {
		log.Fatal(err)
	}
	if answer == "" {
		fmt.Println("no answer within 30 s")
		os.Exit(1)
	}
	fmt.Println(answer)
}
