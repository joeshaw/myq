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
	"net/url"
	"os"
	"strconv"
)

const (
	baseURL = "https://myqexternal.myqdevice.com/api/v4"

	loginEndpoint              = "/User/Validate"
	devicesEndpoint            = "/UserDeviceDetails/Get"
	deviceGetAttributeEndpoint = "/DeviceAttribute/GetDeviceAttribute"
	deviceSetAttributeEndpoint = "/DeviceAttribute/PutDeviceAttribute"

	deviceTypeHub        = 1
	deviceTypeDoorOpener = 2
	deviceTypeGate       = 5
	deviceTypeMyQGarage  = 7
)

const (
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

func stateString(st int) string {
	switch st {
	case 1, 9:
		return StateOpen
	case 2:
		return StateClosed
	case 3:
		return StateStopped
	case 4:
		return StateOpening
	case 5:
		return StateClosing
	default:
		return StateUnknown
	}
}

func stateInt(st string) int {
	switch st {
	case StateOpen:
		return 1
	case StateClosed:
		return 2
	case StateStopped:
		return 3
	case StateOpening:
		return 4
	case StateClosing:
		return 5
	default:
		return 0
	}
}

// Session represents an authenticated session to the MyQ service.
type Session struct {
	Username string
	Password string
	Brand    string

	appID string
	token string
}

// Device defines a MyQ device
type Device struct {
	DeviceID     string
	SerialNumber string
	Type         string
	Name         string
	Desc         string
	State        string
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
	if Debug {
		fmt.Fprintf(os.Stderr, "%s %s\n", req.Method, req.URL.String())
	}

	if req.Body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.token != "" {
		req.Header.Set("SecurityToken", s.token)
	}
	req.Header.Set("MyQApplicationId", s.appID)

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

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("received HTTP status code %d", resp.StatusCode)
	}

	if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
		return err
	}

	switch {
	case target.returnCode() == "-3333":
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
		"username": s.Username,
		"password": s.Password,
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

// Devices returns the list of MyQ devices
func (s *Session) Devices() ([]Device, error) {
	req, err := http.NewRequest("GET", baseURL+devicesEndpoint, nil)
	if err != nil {
		return nil, err
	}

	type device struct {
		MyQDeviceId           int
		ParentMyQDeviceID     int
		MyQDeviceTypeId       int
		MyQDeviceTypeName     string
		RegistrationDateTime  string
		SerialNumber          string
		UserName              string
		UserCountryId         int
		ChildrenMyQDeviceIds  string
		UpdatedBy             string
		UpdatedDate           string
		ConnectServerDeviceId string
		Attributes            []struct {
			MyQDeviceTypeAttributeId int
			Value                    string
			UpdatedTime              string
			IsDeviceProperty         bool
			AttributeDisplayName     string
			IsPersistent             bool
			IsTimeSeries             bool
			IsGlobal                 bool
			UpdatedDate              string
		}
	}

	var body struct {
		baseResponse
		Devices []device
	}

	if err := s.apiRequestWithRetry(req, &body); err != nil {
		return nil, err
	}

	devices := make([]Device, len(body.Devices))
	for i := 0; i < len(body.Devices); i++ {
		devices[i] = Device{
			DeviceID:     fmt.Sprint(body.Devices[i].MyQDeviceId),
			SerialNumber: body.Devices[i].SerialNumber,
			Type:         body.Devices[i].MyQDeviceTypeName,
			State:        StateUnknown,
		}

		for _, attr := range body.Devices[i].Attributes {
			switch attr.AttributeDisplayName {
			case "name":
				devices[i].Name = attr.Value

			case "desc":
				devices[i].Desc = attr.Value

			case "doorstate":
				st, _ := strconv.Atoi(attr.Value)
				devices[i].State = stateString(st)
			}
		}
	}

	return devices, nil
}

// DeviceState returns the device state (open, closed, etc.) for the
// provided device ID
func (s *Session) DeviceState(deviceID string) (string, error) {
	v := url.Values{}
	v.Set("AttributeName", "doorstate")
	v.Set("MyQDeviceId", deviceID)

	u, _ := url.Parse(baseURL + deviceGetAttributeEndpoint)
	u.RawQuery = v.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return StateUnknown, err
	}

	var body struct {
		baseResponse
		AttributeValue string
	}

	if err := s.apiRequestWithRetry(req, &body); err != nil {
		return StateUnknown, err
	}

	st, _ := strconv.Atoi(body.AttributeValue)
	return stateString(st), nil
}

// SetDeviceState sets the device state (open or closed) for the
// provided device ID
func (s *Session) SetDeviceState(deviceID string, state string) error {
	st := stateInt(state)
	if st == 0 {
		return fmt.Errorf("invalid state %s", state)
	}

	data, err := json.Marshal(map[string]string{
		"AttributeName":  "desireddoorstate",
		"MyQDeviceId":    deviceID,
		"AttributeValue": fmt.Sprint(st),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("PUT", baseURL+deviceSetAttributeEndpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}

	var body baseResponse
	if err := s.apiRequestWithRetry(req, &body); err != nil {
		return err
	}

	return nil
}
