package rns

import (
	"context"

	"github.com/Fishwaldo/go-logadapter"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
)

type Client struct {
	ClientID string
	Bucket   string
}

type RNSServerImpl interface {
	LookupClient(string) (Client, error)
	Open(context.Context, OpenRepoOp) (OpenRepoResult, Client, error)
	Stat(context.Context, Client, StatOp) (StatResult, error)
	Mkdir(context.Context, Client, MkdirOp) (MkdirResult, error)
	Save(context.Context, Client, SaveOp) (SaveResult, error)
	List(context.Context, Client, ListOp) (ListResult, error)
	Load(context.Context, Client, LoadOp) (LoadResult, error)
	Remove(context.Context, Client, RemoveOp) (RemoveResult, error)
	Close(context.Context, Client, CloseOp) (CloseResult, error)
}

type RNSServer struct {
	impl   RNSServerImpl
	log    logadapter.Logger
	client *ResticNatsClient
}

var (
	errRNSServerError = errors.New("Server Failure")
)

func NewRNSServer(impl RNSServerImpl, rnsclient *ResticNatsClient, log logadapter.Logger) (RNSServer, error) {
	ret := RNSServer{
		impl:   impl,
		client: rnsclient,
		log:    log,
	}
	return ret, nil
}

func (rns *RNSServer) decodeMsg(msg *nats.Msg, vptr interface{}) error {
	if err := rns.client.Encoder.Decode(msg.Subject, msg.Data, vptr); err != nil {
		rns.log.Warn("Decode Failed: %s", err)
		return errRNSServerError
	}
	return nil
}

func (rns *RNSServer) replyResult(ctx context.Context, replyto *nats.Msg, result interface{}) error {
	var err error
	reply := NewRNSReplyMsg(replyto)
	reply.Data, err = rns.client.Encoder.Encode(reply.Reply, result)
	if err != nil {
		rns.log.Warn("Encode Failed: %s", err)
		return errRNSServerError
	}
	/* send it */
	if err = rns.client.ChunkSendReplyMsgWithContext(ctx, replyto, reply); err != nil {
		rns.log.Warn("Send Reply Failed: %s", err)
		return errRNSServerError
	}
	return nil
}

func (rns *RNSServer) ProcessServerMsg(ctx context.Context, msg *nats.Msg) error {
	rns.log.Info("Message: %s", msg.Subject)

	/* get the entire message, if its chunked */
	msg, err := rns.client.ChunkReadRequestMsgWithContext(ctx, msg)
	if err != nil {
		rns.log.Warn("Chunk Read Failed: %s", err)
		return err
	}

	var cmd NatsCommand = NatsCommand(msg.Header.Get(msgHeaderOperation))

	var client Client
	/* first case, if its a open command */
	if cmd == NatsOpenCmd {
		var oo OpenRepoOp
		err := rns.decodeMsg(msg, &oo)
		if err != nil {
			return err
		}
		var or OpenRepoResult
		or, client, err = rns.impl.Open(ctx, oo)
		if err != nil {
			/* its a server specific error */
			rns.log.Warn("Server Open Failed: %s", err)
			/* make sure our result contains a generic error */
			or.Err = errRNSServerError
		}
		return rns.replyResult(ctx, msg, or)
	} else {
		/* any other command should have our CID */
		client, err = rns.impl.LookupClient(msg.Header.Get(msgHeaderClientID))
		if err != nil {
			rns.log.Warn("Client Lookup Failed: %s", err)
			return err
		}
	}
	var cmdResult interface{}

	/* now we can process the rest of our commands */
	switch cmd {
	case NatsStatCmd:
		var so StatOp
		err := rns.decodeMsg(msg, &so)
		if err != nil {
			return err
		}
		cmdResult, err = rns.impl.Stat(ctx, client, so)
		if err != nil {
			sr := cmdResult.(StatResult)
			sr.Err = errRNSServerError
		}
	case NatsMkdirCmd:
		var mo MkdirOp
		err := rns.decodeMsg(msg, &mo)
		if err != nil {
			return err
		}
		cmdResult, err = rns.impl.Mkdir(ctx, client, mo)
		if err != nil {
			sr := cmdResult.(MkdirResult)
			sr.Err = errRNSServerError
		}
	case NatsSaveCmd:
		var so SaveOp
		err := rns.decodeMsg(msg, &so)
		if err != nil {
			return err
		}
		cmdResult, err = rns.impl.Save(ctx, client, so)
		if err != nil {
			sr := cmdResult.(SaveResult)
			sr.Err = errRNSServerError
		}
	case NatsListCmd:
		var lo ListOp
		err := rns.decodeMsg(msg, &lo)
		if err != nil {
			return err
		}
		cmdResult, err = rns.impl.List(ctx, client, lo)
		if err != nil {
			sr := cmdResult.(ListResult)
			sr.Err = errRNSServerError
		}
	case NatsLoadCmd:
		var lo LoadOp
		err := rns.decodeMsg(msg, &lo)
		if err != nil {
			return err
		}
		cmdResult, err = rns.impl.Load(ctx, client, lo)
		if err != nil {
			sr := cmdResult.(LoadResult)
			sr.Err = errRNSServerError
		}
	case NatsRemoveCmd:
		var ro RemoveOp
		err := rns.decodeMsg(msg, &ro)
		if err != nil {
			return err
		}
		cmdResult, err = rns.impl.Remove(ctx, client, ro)
		if err != nil {
			sr := cmdResult.(RemoveResult)
			sr.Err = errRNSServerError
		}
	case NatsCloseCmd:
		var co CloseOp
		err := rns.decodeMsg(msg, &co)
		if err != nil {
			return err
		}
		cmdResult, err = rns.impl.Close(ctx, client, co)
		if err != nil {
			sr := cmdResult.(CloseResult)
			sr.Err = errRNSServerError
		}
	default:
		rns.log.Warn("Got Unknown Command %s", cmd)
		return errRNSServerError
	}

	if err != nil {
		rns.log.Warn("Server %s Failed: %s", cmd, err)
	}
	return rns.replyResult(ctx, msg, cmdResult)
}
