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

type config struct {
	username string
	password string
	brand    string
}

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
	fmt.Fprintf(os.Stderr, "  open              Open device\n")
	fmt.Fprintf(os.Stderr, "  close             Close device\n")
	fmt.Fprintf(os.Stderr, "\n")
}

func main() {
	var cfg config

	flag.StringVar(&cfg.username, "username", "", "MyQ username")
	flag.StringVar(&cfg.password, "password", "", "MyQ password")
	flag.StringVar(&cfg.brand, "brand", "liftmaster", "Equipment brand")
	flag.BoolVar(&myq.Debug, "debug", false, "debug mode")
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	if v := os.Getenv("MYQ_USERNAME"); v != "" && cfg.username == "" {
		cfg.username = v
	}

	if v := os.Getenv("MYQ_PASSWORD"); v != "" && cfg.password == "" {
		cfg.password = v
	}

	if cfg.username == "" {
		fmt.Fprintf(os.Stderr, "ERROR: -username must be provided\n")
		os.Exit(1)
	}

	if cfg.password == "" {
		fmt.Fprintf(os.Stderr, "ERROR: -password must be provided\n")
		os.Exit(1)
	}

	var run func(*myq.Session, []string) error

	cmd, args := strings.ToLower(args[0]), args[1:]
	switch cmd {
	case "devices":
		run = runDevices

	case "open":
		run = runOpen

	case "close":
		run = runClose

	default:
		usage()
		os.Exit(1)
	}

	fmt.Println("Logging into MyQ...")

	s, err := myq.Connect(cfg.username, cfg.password, cfg.brand)
	if err != nil {
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
		fmt.Printf("Device ID %s\n", d.DeviceID)
		if d.Type != "" {
			fmt.Printf("  Type: %s\n", d.Type)
		}
		fmt.Printf("  Name: %s\n", d.Name)
		fmt.Printf("  Description: %s\n", d.Desc)
		fmt.Printf("  Serial Number: %s\n", d.SerialNumber)
		fmt.Printf("  State: %s\n", d.State)
		fmt.Println()
	}

	return nil
}

func openOrClose(s *myq.Session, deviceID string, desiredState string) error {
	if err := s.SetDeviceState(deviceID, desiredState); err != nil {
		return err
	}

	fmt.Printf("Waiting for door to be %s...\n", desiredState)

	var currentState string
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		state, err := s.DeviceState(deviceID)
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
		return errors.New("specify a MyQ device ID")
	}

	return openOrClose(s, args[0], myq.StateOpen)
}

func runClose(s *myq.Session, args []string) error {
	if len(args) == 0 {
		return errors.New("specify a MyQ device ID")
	}

	return openOrClose(s, args[0], myq.StateClosed)
}
