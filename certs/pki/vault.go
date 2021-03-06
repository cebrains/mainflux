// Copyright (c) Mainflux
// SPDX-License-Identifier: Apache-2.0

// Package pki wraps vault client
package pki

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"time"

	"github.com/hashicorp/vault/api"
	"github.com/mainflux/mainflux/pkg/errors"
	"github.com/mitchellh/mapstructure"
)

const (
	issue  = "issue"
	revoke = "revoke"
	apiVer = "v1"
)

var (
	errFailedVaultCertIssue = errors.New("failed to issue vault certificate")
	errFailedCertDecoding   = errors.New("failed to decode response from vault service")
)

type pkiAgent struct {
	token     string
	path      string
	role      string
	host      string
	issueURL  string
	revokeURL string
	client    *api.Client
}

type certReq struct {
	CommonName string `json:"common_name"`
	TTL        string `json:"ttl"`
	KeyBits    int    `json:"key_bits"`
	KeyType    string `json:"key_type"`
}

type certRevokeReq struct {
	SerialNumber string `json:"serial_number"`
}

func NewVaultClient(token, host, path, role string) (Agent, error) {
	conf := &api.Config{
		Address: host,
	}

	client, err := api.NewClient(conf)
	if err != nil {
		return nil, err
	}
	client.SetToken(token)
	p := pkiAgent{
		token:     token,
		host:      host,
		role:      role,
		path:      path,
		client:    client,
		issueURL:  "/" + apiVer + "/" + path + "/" + issue + "/" + role,
		revokeURL: "/" + apiVer + "/" + path + "/" + revoke,
	}
	return &p, nil
}

func (p *pkiAgent) IssueCert(cn string, ttl, keyType string, keyBits int) (Cert, error) {
	cReq := certReq{
		CommonName: cn,
		TTL:        ttl,
		KeyBits:    keyBits,
		KeyType:    keyType,
	}

	r := p.client.NewRequest("POST", p.issueURL)
	if err := r.SetJSONBody(cReq); err != nil {
		return Cert{}, err
	}

	resp, err := p.client.RawRequest(r)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return Cert{}, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		_, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return Cert{}, err
		}
		return Cert{}, errors.Wrap(errFailedVaultCertIssue, err)
	}

	s, _ := api.ParseSecret(resp.Body)
	cert := Cert{}

	if err = mapstructure.Decode(s.Data, &cert); err != nil {
		return Cert{}, errors.Wrap(errFailedCertDecoding, err)
	}

	// Expire time calc must be revised value doesnt look correct
	exp, err := s.Data["expiration"].(json.Number).Float64()
	if err != nil {
		return cert, err
	}
	expTime := time.Unix(0, int64(exp)*int64(time.Millisecond))
	cert.Expire = expTime
	return cert, nil

}

func (p *pkiAgent) Revoke(serial string) (Revoke, error) {
	cReq := certRevokeReq{
		SerialNumber: serial,
	}

	r := p.client.NewRequest("POST", p.revokeURL)
	if err := r.SetJSONBody(cReq); err != nil {
		return Revoke{}, err
	}

	resp, err := p.client.RawRequest(r)
	if resp != nil {
		defer resp.Body.Close()
	}

	if err != nil {
		return Revoke{}, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		_, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return Revoke{}, err
		}
		return Revoke{}, errors.Wrap(errFailedVaultCertIssue, err)
	}

	s, err := api.ParseSecret(resp.Body)
	if err != nil {
		return Revoke{}, err
	}

	rev, err := s.Data["revocation_time"].(json.Number).Float64()
	if err != nil {
		return Revoke{}, err
	}
	revTime := time.Unix(0, int64(rev)*int64(time.Millisecond))
	return Revoke{
		RevocationTime: revTime,
	}, nil

}
