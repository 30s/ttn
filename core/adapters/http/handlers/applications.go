// Copyright © 2016 The Things Network
// Use of this source code is governed by the MIT license that can be found in the LICENSE file.

package handlers

import (
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/TheThingsNetwork/ttn/core"
	. "github.com/TheThingsNetwork/ttn/core/adapters/http"
	"github.com/TheThingsNetwork/ttn/utils/errors"
	"github.com/brocaar/lorawan"
)

// Applications defines a handler to handle application registration on a component.
//
// It listens to request of the form: [PUT] /applications/:appEUI
// where appEUI is a 8 bytes hex-encoded address.
//
// It expects a Content-Type = application/json
//
// It also looks for params:
//
// - app_url (http address as string)
//
// It fails with an http 400 Bad Request. if one of the parameter is missing or invalid
// It succeeds with an http 2xx if the request is valid (the response status is under the
// ackNacker responsibility).
// It can possibly fails with another status depending of the AckNacker response.
//
// The PubSub handler generates registration where:
// - AppEUI is available
// - Recipient can be interpreted as an HttpRecipient (Url + Method)
type Applications struct{}

// URL implements the http.Handler interface
func (p Applications) URL() string {
	return "/applications/"
}

// Handle implements the http.Handler interface
func (p Applications) Handle(w http.ResponseWriter, chpkt chan<- PktReq, chreg chan<- RegReq, req *http.Request) {
	// Check the http method
	if req.Method != "PUT" {
		err := errors.New(errors.Structural, "Unreckognized HTTP method. Please use [PUT] to register a device")
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte(err.Error()))
		return
	}

	// Parse body and query params
	registration, err := p.parse(req)
	if err != nil {
		BadRequest(w, err.Error())
		return
	}

	// Send the registration and wait for ack / nack
	chresp := make(chan MsgRes)
	chreg <- RegReq{Registration: registration, Chresp: chresp}
	r, ok := <-chresp
	if !ok {
		BadRequest(w, "Core server not responding")
		return
	}
	w.WriteHeader(r.StatusCode)
	w.Write(r.Content)
}

// parse extracts params from the request and fails if the request is invalid.
func (p Applications) parse(req *http.Request) (core.Registration, error) {
	// Check Content-type
	if req.Header.Get("Content-Type") != "application/json" {
		return applicationsRegistration{}, errors.New(errors.Structural, "Received invalid content-type in request")
	}

	// Check the query parameter
	reg := regexp.MustCompile("applications/([a-fA-F0-9]{16})$") // 8-bytes, hex-encoded -> 16 chars
	query := reg.FindStringSubmatch(req.RequestURI)
	if len(query) < 2 {
		return applicationsRegistration{}, errors.New(errors.Structural, "Incorrect application identifier format")
	}
	appEUI, err := hex.DecodeString(query[1])
	if err != nil {
		return applicationsRegistration{}, errors.New(errors.Structural, err)
	}

	// Check configuration in body
	body := make([]byte, req.ContentLength)
	n, err := req.Body.Read(body)
	if err != nil && err != io.EOF {
		return applicationsRegistration{}, errors.New(errors.Structural, err)
	}
	defer req.Body.Close()
	params := &struct {
		URL string `json:"app_url"`
	}{}
	if err := json.Unmarshal(body[:n], params); err != nil {
		return applicationsRegistration{}, errors.New(errors.Structural, "Unable to unmarshal the request body")
	}

	// Verify each request parameter
	params.URL = strings.Trim(params.URL, " ")
	if len(params.URL) <= 0 {
		return applicationsRegistration{}, errors.New(errors.Structural, "Incorrect application url")
	}

	// Create actual registration
	registration := applicationsRegistration{
		recipient: NewRecipient(params.URL, "PUT"),
		appEUI:    lorawan.EUI64{},
	}
	copy(registration.appEUI[:], appEUI[:])
	return registration, nil
}