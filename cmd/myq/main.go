package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/joeshaw/myq"
)

func usage() {
	fmt.Fprintf(os.Stderr, "USAGE\n")
	fmt.Fprintf(os.Stderr, "  %s <mode> [flags]\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "FLAGS\n")
	flag.VisitAll(func(f *flag.Flag) {
		fmt.Fprintf(os.Stderr, "  -%s %s\n", f.Name, f.Usage)
	})
	fmt.Fprintf(os.Stderr, "\n")
	fmt.Fprintf(os.Stderr, "COMMANDS\n")
	fmt.Fprintf(os.Stderr, "  devices           Print MyQ devices\n")
	fmt.Fprintf(os.Stderr, "  state             Print current door state for a device\n")
	fmt.Fprintf(os.Stderr, "  open              Open device\n")
	fmt.Fprintf(os.Stderr, "  close             Close device\n")
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {
	s := &myq.Session{}

	flag.StringVar(&s.Username, "username", "", "MyQ username")
	flag.StringVar(&s.Password, "password", "", "MyQ password")
	flag.StringVar(&s.Brand, "brand", "liftmaster", "Equipment brand")
	flag.BoolVar(&myq.Debug, "debug", false, "debug mode")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	if v := os.Getenv("MYQ_USERNAME"); v != "" && s.Username == "" {
		s.Username = v
	}

	if v := os.Getenv("MYQ_PASSWORD"); v != "" && s.Password == "" {
		s.Password = v
	}

	if s.Username == "" {
		fmt.Fprintf(os.Stderr, "ERROR: -username must be provided\n")
		os.Exit(1)
	}

	if s.Password == "" {
		fmt.Fprintf(os.Stderr, "ERROR: -password must be provided\n")
		os.Exit(1)
	}

	var run func(*myq.Session, []string) error

	cmd, args := strings.ToLower(args[0]), args[1:]
	switch cmd {
	case "devices":
		run = runDevices

	case "state":
		run = runState

	case "open":
		run = runOpen

	case "close":
		run = runClose

	default:
		usage()
		os.Exit(1)
	}

	fmt.Println("Logging into MyQ...")

	if err := s.Login(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}

	if err := run(s, args); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func runDevices(s *myq.Session, args []string) error {
	fmt.Println("Requesting devices from MyQ...")

	devices, err := s.Devices()
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		fmt.Println("No devices found.")
		return nil
	}

	for _, d := range devices {
		fmt.Printf("Device %s\n", d.SerialNumber)
		fmt.Printf("  Name: %s\n", d.Name)
		fmt.Printf("  Type: %s\n", d.Type)
		if d.DoorState != "" {
			fmt.Printf("  Door State: %s\n", d.DoorState)
		}
		fmt.Println()
	}

	return nil
}

func runState(s *myq.Session, args []string) error {
	if len(args) == 0 {
		return errors.New("specify a MyQ device serial number")
	}

	serialNumber := args[0]

	state, err := s.DeviceState(serialNumber)
	if err != nil {
		return err
	}

	if state == "" {
		fmt.Printf("Device %s has no door state\n", serialNumber)
	} else {
		fmt.Printf("Device %s is %s\n", serialNumber, state)
	}
	return nil
}

func openOrClose(s *myq.Session, serialNumber string, action string) error {
	var desiredState string
	switch action {
	case myq.ActionOpen:
		desiredState = myq.StateOpen
	case myq.ActionClose:
		desiredState = myq.StateClosed
	}

	if err := s.SetDoorState(serialNumber, action); err != nil {
		return err
	}

	fmt.Printf("Waiting for door to be %s...\n", desiredState)

	var currentState string
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		state, err := s.DeviceState(serialNumber)
		if err != nil {
			return err
		}
		if state != currentState {
			if currentState != "" {
				fmt.Printf("Door state changed to %s\n", state)
			}
			currentState = state
		}
		if currentState == desiredState {
			break
		}
		time.Sleep(5 * time.Second)
	}

	if currentState != desiredState {
		return fmt.Errorf("timed out waiting for door to be %s", desiredState)
	}

	return nil
}

func runOpen(s *myq.Session, args []string) error {
	if len(args) == 0 {
		return errors.New("specify a MyQ device serial number")
	}

	return openOrClose(s, args[0], myq.ActionOpen)
}

func runClose(s *myq.Session, args []string) error {
	if len(args) == 0 {
		return errors.New("specify a MyQ device serial number")
	}

	return openOrClose(s, args[0], myq.ActionClose)
}
