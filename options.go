package rns

import (
	"crypto/tls"
	"os"
	"time"

	"github.com/Fishwaldo/go-logadapter"
	"github.com/nats-io/nats.go"
)

type RNSOptions func (*ResticNatsClient) error

func WithCredentials(credfile string) RNSOptions {
	return func (rns *ResticNatsClient) error {
		f, err := os.Open(credfile)
		if err != nil {
			return err
		}
		f.Close()
		rns.natsoptions = append(rns.natsoptions, nats.UserCredentials(credfile))
		return nil
	}
}

func WithUserPass(username, password string) RNSOptions {
	return func(rns *ResticNatsClient) error {
		rns.natsoptions = append(rns.natsoptions, nats.UserInfo(username, password))
		return nil
	}
}

func WithName(name string) RNSOptions {
	return func(rns *ResticNatsClient) error {
		rns.natsoptions = append(rns.natsoptions, nats.Name(name))
		return nil
	}
}

func WithPingInterval(t time.Duration) RNSOptions {
	return func(rns *ResticNatsClient) error {
		rns.natsoptions = append(rns.natsoptions, nats.PingInterval(t))
		return nil
	}
}

func WithTLSOptions(tls *tls.Config) RNSOptions {
	return func(rns *ResticNatsClient) error {
		rns.natsoptions = append(rns.natsoptions, nats.Secure(tls))
		return nil
	}
}

func WithLogger(log logadapter.Logger) RNSOptions {
	return func(rns *ResticNatsClient) error {
		rns.logger = log
		return nil
	}
}
func WithServer() RNSOptions {
	return func(rns *ResticNatsClient) error {
		rns.server = true
		return nil
	}
}