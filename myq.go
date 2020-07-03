package myq

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
)

const (
	baseURL = "https://api.myqdevice.com"

	loginEndpoint       = "/api/v5/Login"
	accountInfoEndpoint = "/api/v5/My?expand=account"

	// Parameter is account ID
	devicesEndpointFmt = "/api/v5.1/Accounts/%s/Devices"

	// Parameters are account ID and device serial number
	deviceEndpointFmt        = "/api/v5.1/Accounts/%s/Devices/%s"
	deviceActionsEndpointFmt = "/api/v5.1/Accounts/%s/Devices/%s/actions"
)

const (
	ActionClose = "close"
	ActionOpen  = "open"

	StateUnknown = "unknown"
	StateOpen    = "open"
	StateClosed  = "closed"
	StateStopped = "stopped"
	StateOpening = "opening"
	StateClosing = "closing"
)

var (
	// Debug indiciates whether to log HTTP responses to stderr
	Debug = false

	// ErrNotLoggedIn is returned whenever an operation is run the
	// user has not logged in
	ErrNotLoggedIn = errors.New("not logged in")

	appIDs = map[string]string{
		"liftmaster":  "Vj8pQggXLhLy0WHahglCD4N1nAkkXQtGYpq2HrHD7H1nvmbT55KqtN6RSF4ILB/i",
		"chamberlain": "OA9I/hgmPHFp9RYKJqCKfwnhh28uqLJzZ9KOJf1DXoo8N2XAaVX6A1wcLYyWsnnv",
		"craftsman":   "YmiMRRS1juXdSd0KWsuKtHmQvh5RftEp5iewHdCvsNB77FnQbY+vjCVn2nMdIeN8",
		"merlin":      "3004cac4e920426c823fa6c2ecf0cc28ef7d4a7b74b6470f8f0d94d6c39eb718",
	}
)

// Session represents an authenticated session to the MyQ service.
type Session struct {
	Username string
	Password string
	Brand    string

	appID     string
	token     string
	accountID string
}

// Device defines a MyQ device
type Device struct {
	SerialNumber string
	Type         string
	Name         string
	DoorState    string
}

type response interface {
	returnCode() string
	errorMessage() string
}

type baseResponse struct {
	ReturnCode   string
	ErrorMessage string
}

func (r *baseResponse) returnCode() string {
	return r.ReturnCode
}

func (r *baseResponse) errorMessage() string {
	return r.ErrorMessage
}

func (s *Session) apiRequest(req *http.Request, target response) error {
	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("SecurityToken", s.token)
	}
	req.Header.Set("MyQApplicationId", s.appID)

	if Debug {
		fmt.Fprintf(os.Stderr, "%s %s\n", req.Method, req.URL.String())
		d, _ := httputil.DumpRequest(req, true)
		fmt.Fprintln(os.Stderr, string(d))
		fmt.Fprintln(os.Stderr)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(ioutil.Discard, resp.Body)
		resp.Body.Close()
	}()

	if Debug {
		d, _ := httputil.DumpResponse(resp, true)
		fmt.Fprintln(os.Stderr, string(d))
		fmt.Fprintln(os.Stderr)
	}

	if resp.StatusCode == http.StatusNoContent {
		return nil
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received HTTP status code %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return err
	}

	switch {
	case target.returnCode() == "-3333", target.returnCode() == "216":
		return ErrNotLoggedIn

	case target.errorMessage() != "":
		return errors.New(target.errorMessage())
	}

	return nil
}

func (s *Session) apiRequestWithRetry(req *http.Request, target response) error {
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
	appID, ok := appIDs[s.Brand]
	if !ok {
		return errors.New("unknown brand type")
	}
	s.appID = appID

	data, err := json.Marshal(map[string]string{
		"Username": s.Username,
		"Password": s.Password,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", baseURL+loginEndpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}

	var body struct {
		baseResponse
		SecurityToken string
	}

	if err := s.apiRequest(req, &body); err != nil {
		return err
	}

	s.token = body.SecurityToken
	return nil
}

func (s *Session) fillAccountID() error {
	req, err := http.NewRequest("GET", baseURL+accountInfoEndpoint, nil)
	if err != nil {
		return err
	}

	var body struct {
		baseResponse
		Account struct {
			ID string `json:"Id"`
		}
	}

	if err := s.apiRequestWithRetry(req, &body); err != nil {
		return err
	}

	if body.Account.ID == "" {
		return errors.New("empty account ID")
	}

	s.accountID = body.Account.ID
	return nil
}

// Devices returns the list of MyQ devices
func (s *Session) Devices() ([]Device, error) {
	if s.accountID == "" {
		if err := s.fillAccountID(); err != nil {
			return nil, err
		}
	}

	devicesEndpoint := fmt.Sprintf(devicesEndpointFmt, s.accountID)
	req, err := http.NewRequest("GET", baseURL+devicesEndpoint, nil)
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
		baseResponse
		Items []item `json:"items"`
	}

	if err := s.apiRequestWithRetry(req, &body); err != nil {
		return nil, err
	}

	devices := make([]Device, len(body.Items))
	for i := range body.Items {
		devices[i] = Device{
			SerialNumber: body.Items[i].SerialNumber,
			Type:         body.Items[i].DeviceType,
			Name:         body.Items[i].Name,
			DoorState:    body.Items[i].State.DoorState,
		}
	}

	return devices, nil
}

// DeviceState returns the device state (open, closed, etc.) for the
// provided device serial number
func (s *Session) DeviceState(serialNumber string) (string, error) {
	if s.accountID == "" {
		if err := s.fillAccountID(); err != nil {
			return "", err
		}
	}

	deviceEndpoint := fmt.Sprintf(deviceEndpointFmt, s.accountID, serialNumber)
	req, err := http.NewRequest("GET", baseURL+deviceEndpoint, nil)
	if err != nil {
		return "", err
	}

	var body struct {
		baseResponse
		SerialNumber string `json:"serial_number"`
		DeviceType   string `json:"device_type"`
		Name         string `json:"name"`
		State        struct {
			DoorState string `json:"door_state"`
		} `json:"state"`
	}

	if err := s.apiRequestWithRetry(req, &body); err != nil {
		return "", err
	}

	return body.State.DoorState, nil
}

// SetDoorState sets the target door state (open or closed) for the
// provided device serial number
func (s *Session) SetDoorState(serialNumber string, action string) error {
	if s.accountID == "" {
		if err := s.fillAccountID(); err != nil {
			return err
		}
	}

	deviceActionsEndpoint := fmt.Sprintf(deviceActionsEndpointFmt, s.accountID, serialNumber)
	data := fmt.Sprintf(`{"action_type":"%s"}`, action)
	req, err := http.NewRequest("PUT", baseURL+deviceActionsEndpoint, strings.NewReader(data))
	if err != nil {
		return err
	}

	var body struct {
		baseResponse
	}

	if err := s.apiRequestWithRetry(req, &body); err != nil {
		return err
	}

	return nil
}
