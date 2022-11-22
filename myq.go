package myq

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
)

const (
	accountsEndpoint = "https://accounts.myq-cloud.com/api/v6.0/accounts"

	// Parameter is account ID
	devicesEndpointFmt = "https://devices.myq-cloud.com/api/v5.2/Accounts/%s/Devices"

	// Parameters are account ID and device serial number
	deviceEndpointFmt = "https://devices.myq-cloud.com/api/v5.2/Accounts/%s/Devices/%s"

	// Parameters are account ID, device serial number, and action (open or close)
	deviceActionsEndpointFmt = "https://account-devices-gdo.myq-cloud.com/api/v5.2/Accounts/%s/door_openers/%s/%s"
)

const (
	ActionClose = "close"
	ActionOpen  = "open"

	StateUnknown = "unknown"
	StateOpen    = "open"
	StateClosed  = "closed"
	StateStopped = "stopped"
)

var (
	// Debug indiciates whether to log HTTP responses to stderr
	Debug = false

	// ErrNotLoggedIn is returned whenever an operation is run the
	// user has not logged in
	ErrNotLoggedIn = errors.New("not logged in")
)

// Session represents an authenticated session to the MyQ service.
type Session struct {
	Username string
	Password string

	token    string
	accounts []*Account
}

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Device defines a MyQ device
type Device struct {
	Account      *Account
	SerialNumber string
	Type         string
	Name         string
	DoorState    string
}

type errorResponse struct {
	StatusCode int `json:"-"`

	Message     string
	Description string
}

func (e *errorResponse) Error() string {
	if e.Description != "" {
		return e.Message + ": " + e.Description
	}
	return e.Message
}

func isStatus(err error, code int) bool {
	e, ok := err.(*errorResponse)
	return ok && e.StatusCode == code
}

func drain(rc io.ReadCloser) {
	io.Copy(ioutil.Discard, rc)
	rc.Close()
}

func doRequest(client *http.Client, req *http.Request) (*http.Response, error) {
	if Debug {
		d, _ := httputil.DumpRequestOut(req, true)
		fmt.Fprintln(os.Stderr, string(d))
		fmt.Fprintln(os.Stderr)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if Debug {
		d, _ := httputil.DumpResponse(resp, true)
		fmt.Fprintln(os.Stderr, string(d))
		fmt.Fprintln(os.Stderr)
	}

	return resp, nil
}

func (s *Session) apiRequest(req *http.Request, target interface{}) error {
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}

	resp, err := doRequest(http.DefaultClient, req)
	if err != nil {
		return err
	}
	defer drain(resp.Body)

	switch resp.StatusCode {
	case http.StatusOK:
		return json.NewDecoder(resp.Body).Decode(target)

	case http.StatusNoContent, http.StatusAccepted:
		return nil

	case http.StatusUnauthorized:
		return ErrNotLoggedIn

	default:
		var errResp errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			errResp.Message = fmt.Sprintf("received HTTP status code %d", resp.StatusCode)
		}
		errResp.StatusCode = resp.StatusCode
		return &errResp
	}
}

func (s *Session) apiRequestWithRetry(req *http.Request, target interface{}) error {
	if err := s.apiRequest(req, target); err == ErrNotLoggedIn {
		if err := s.Login(); err != nil {
			return err
		}

		return s.apiRequest(req, target)
	} else {
		return err
	}
}

// Login establishes an authenticated Session with the MyQ service
func (s *Session) Login() error {
	o, err := newOAuth()
	if err != nil {
		return err
	}

	u, err := o.authorize()
	if err != nil {
		return err
	}

	u, err = o.login(u, s.Username, s.Password)
	if err != nil {
		return err
	}

	u, err = o.callback(u)
	if err != nil {
		return err
	}

	token, err := o.token(u)
	if err != nil {
		return err
	}

	s.token = token
	return nil
}

func (s *Session) fillAccounts() error {
	if len(s.accounts) > 0 {
		return nil
	}

	req, err := http.NewRequest("GET", accountsEndpoint, nil)
	if err != nil {
		return err
	}

	var jsonResponse struct {
		Accounts []*Account `json:"accounts"`
	}

	if err := s.apiRequestWithRetry(req, &jsonResponse); err != nil {
		return err
	}

	s.accounts = jsonResponse.Accounts
	return nil
}

// Devices returns the list of MyQ devices
func (s *Session) Devices() ([]Device, error) {
	if err := s.fillAccounts(); err != nil {
		return nil, err
	}

	var devices []Device

	for _, acct := range s.accounts {
		devicesEndpoint := fmt.Sprintf(devicesEndpointFmt, acct.ID)
		req, err := http.NewRequest("GET", devicesEndpoint, nil)
		if err != nil {
			return nil, err
		}

		type item struct {
			SerialNumber string `json:"serial_number"`
			DeviceType   string `json:"device_type"`
			Name         string `json:"name"`
			State        struct {
				DoorState string `json:"door_state"`
			} `json:"state"`
		}

		var body struct {
			Items []item `json:"items"`
		}

		if err := s.apiRequestWithRetry(req, &body); err != nil {
			return nil, err
		}

		for i := range body.Items {
			devices = append(devices, Device{
				Account:      acct,
				SerialNumber: body.Items[i].SerialNumber,
				Type:         body.Items[i].DeviceType,
				Name:         body.Items[i].Name,
				DoorState:    body.Items[i].State.DoorState,
			})
		}
	}

	return devices, nil
}

// DeviceState returns the device state (open, closed, etc.) for the
// provided device serial number
func (s *Session) DeviceState(serialNumber string) (string, error) {
	if err := s.fillAccounts(); err != nil {
		return "", err
	}

	for _, acct := range s.accounts {
		deviceEndpoint := fmt.Sprintf(deviceEndpointFmt, acct.ID, serialNumber)
		req, err := http.NewRequest("GET", deviceEndpoint, nil)
		if err != nil {
			return "", err
		}

		var body struct {
			SerialNumber string `json:"serial_number"`
			DeviceType   string `json:"device_type"`
			Name         string `json:"name"`
			State        struct {
				DoorState string `json:"door_state"`
			} `json:"state"`
		}

		if err := s.apiRequestWithRetry(req, &body); err != nil {
			if isStatus(err, http.StatusNotFound) {
				continue
			}
			return "", err
		}

		return body.State.DoorState, nil
	}

	return "", fmt.Errorf("device %s not found", serialNumber)
}

// SetDoorState sets the target door state (open or closed) for the
// provided device serial number
func (s *Session) SetDoorState(serialNumber string, action string) error {
	if err := s.fillAccounts(); err != nil {
		return err
	}

	for _, acct := range s.accounts {
		deviceActionsEndpoint := fmt.Sprintf(deviceActionsEndpointFmt, acct.ID, serialNumber, action)
		req, err := http.NewRequest("PUT", deviceActionsEndpoint, nil)
		if err != nil {
			return err
		}

		var body struct{}

		if err := s.apiRequestWithRetry(req, &body); err != nil {
			if isStatus(err, http.StatusNotFound) {
				continue
			}
			return err
		}

		return nil
	}

	return fmt.Errorf("device %s not found", serialNumber)
}
