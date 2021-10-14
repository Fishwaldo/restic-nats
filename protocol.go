package rns

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/Fishwaldo/go-logadapter"
	stdlogger "github.com/Fishwaldo/go-logadapter/loggers/std"
	"github.com/nats-io/nats.go"
	"github.com/pkg/errors"
)

type ResticNatsClient struct {
	Conn        *nats.Conn
	natsoptions []nats.Option
	Encoder     nats.Encoder
	bucket      string
	logger      logadapter.Logger
	server      bool
	clientid	string
}

//header Key Constant Strings for our messages
const (
	msgHeaderID           string = "X-RNS-MSGID"
	msgHeaderChunk        string = "X-RNS-CHUNKS"
	msgHeaderChunkSubject string = "X-RNS-CHUNK-SUBJECT"
	msgHeaderChunkSeq     string = "X-RNS-CHUNKS-SEQ"
	msgHeaderOperation    string = "X-RNS-OP"
	msgHeaderVersion      string = "X-RNS-VERSION"
	msgHeaderClientID	  string = "X-RNS-CLIENTID"
	msgHeaderNRI          string = "Nats-Request-Info"
)

//copyHeader copies the relevent header firles from our source to message
//to the destination message
func copyHeader(msg *nats.Msg) (hdr nats.Header) {
	hdr = make(nats.Header)
	hdr.Add(msgHeaderID, msg.Header.Get(msgHeaderID))
	hdr.Add(msgHeaderChunk, msg.Header.Get(msgHeaderChunk))
	hdr.Add(msgHeaderOperation, msg.Header.Get(msgHeaderOperation))
	return hdr
}

//nriT is the Nats-Request-Info header fields. Used to detect which
//account sent this message
type nriT struct {
	//Acc The Account this message come from
	Acc string `json:"acc"`
	//Round Trip Time
	Rtt int `json:"rtt"`
}

//getNRI gets the nriT from the Nats-Request-Info Header
func getNRI(msg *nats.Msg) (*nriT, bool) {
	nri := msg.Header.Get(msgHeaderNRI)
	if nri == "" {
		return nil, false
	}
	var res nriT
	if err := json.Unmarshal([]byte(nri), &res); err != nil {
		return nil, false
	}
	return &res, true
}

// NewRNSClientMsg Returns a New RNS Client Message (for each "Transaction")
func NewRNSClientMsg(operation NatsCommand) *nats.Msg {
	msg := nats.NewMsg(fmt.Sprintf("repo.commands.%s", operation))
	msg.Header.Set(msgHeaderID, randStringBytesMaskImprSrcSB(16))
	msg.Header.Set(msgHeaderOperation, string(operation))
	msg.Header.Set(msgHeaderVersion, "1")
	return msg
}

// NewRNSClientMsg Returns a New RNS Client Message (for each "Transaction")
func NewRNSReplyMsg(replyto *nats.Msg) *nats.Msg {
	msg := nats.NewMsg(replyto.Reply)
	msg.Header.Set(msgHeaderID, replyto.Header.Get(msgHeaderID))
	msg.Header.Set(msgHeaderOperation, replyto.Header.Get(msgHeaderOperation))
	msg.Header.Set(msgHeaderVersion, "1")
	return msg
}



func New(server url.URL, opts ...RNSOptions) (*ResticNatsClient, error) {
	var rns *ResticNatsClient
	var err error
	rns = &ResticNatsClient{logger: stdlogger.DefaultLogger(), natsoptions: make([]nats.Option, 0)}

	if len(server.User.Username()) > 0 {
		if pass, ok := server.User.Password(); ok {
			opts = append(opts, WithUserPass(server.User.Username(), pass))
		} else {
			return nil, errors.New("No Password Supplied")
		}
	}

	for _, opt := range opts {
		if err := opt(rns); err != nil {
			return nil, errors.Wrap(err, "Open Failed")
		}
	}

	if !rns.server {
		if len(server.Path) == 0 {
			return nil, errors.New("No Bucket Specified")
		} else {
			parts := strings.Split(server.Path, "/")
			if parts[1] == "" {
				return nil, errors.New("Invalid Bucket Specified")
			}
			rns.bucket = parts[1]
		}
	}

	if server.IsAbs() {
		server.Scheme = "nats"
	}
	if server.Port() == "" {
		return nil, errors.New("No Port Specified")
	}

	rns.Conn, err = nats.Connect(server.String(), rns.natsoptions...)
	if err != nil {
		return nil, err
	}

	if size := rns.Conn.MaxPayload(); size < 8388608 {
		return nil, errors.New("NATS Server Max Payload Size is below 8Mb")
	}

	if !rns.Conn.HeadersSupported() {
		return nil, errors.New("server does not support Headers")
	}

	rns.Encoder = nats.EncoderForType("gob")
	if rns.Encoder == nil {
		return nil, errors.New("Can't Load Nats Encoder")
	}
	return rns, nil
}

func (rns *ResticNatsClient) sendOperation(ctx context.Context, msg *nats.Msg, send interface{}, result interface{}) error {
	var err error
	if msg.Data, err = rns.Encoder.Encode(msg.Subject, send); err != nil {
		return errors.Wrap(err, "Encode Failed")
	}
	msg.Header.Add(msgHeaderClientID, rns.clientid)
	msg.Reply = nats.NewInbox()

	var reply *nats.Msg
	if reply, err = rns.ChunkSendRequestMsgWithContext(ctx, msg); err != nil {
		return errors.Wrapf(err, "ChunkRequestMsgWithContext Error: %d", len(msg.Data))
	}

	if len(reply.Data) > 0 {
		if err := rns.Encoder.Decode(reply.Subject, reply.Data, result); err != nil {
			return errors.Wrapf(err, "Decode Failed")
		}
	}
	return nil
}

//ChunkSendReplyMsgWithContext - send a reply to a message, chunking this reply if its Data exceeds the NATS server max payload length
//ctx - Context
//replyto The Message we are replying to
//msg the actual message we want to send
//log Custom Logger
func (rns *ResticNatsClient) ChunkSendReplyMsgWithContext(ctx context.Context, replyto *nats.Msg, msg *nats.Msg) error {
	if len(msg.Header.Get(msgHeaderID)) == 0 {
		return errors.New("MessageID Not Set")
	}

	//Get the max payload size and use 95% as our limit (to account for any additioanl Meta data)
	var maxchunksize int = int(0.95 * float32(rns.Conn.MaxPayload()))
	datasize := len(msg.Data)
	rns.logger.Debug("ChunkSendReplyMsgWithContext: MsgID %s - Headers %s Size: %d", msg.Header.Get(msgHeaderID), msg.Header, len(msg.Data))

	//if the Data is smaller than our payload limit, we can just send it over without chunking
	if len(msg.Data) < maxchunksize {
		/* data is less then our maxchunksize, so we can just send it */
		rns.logger.Trace("ChunkSendReplyMsgWithContext: Short Reply Message %s", msg.Header.Get(msgHeaderID))
		// as this is a reply, we don't have anything coming back...
		err := replyto.RespondMsg(msg)
		return errors.Wrap(err, "Short Reply Message Send Failure")
	}

	/* We need to Split the Data into Chunks
	 * The first Chunk will be sent to the replyto Subject and include a Header
	 * indicating this is a chunked message.
	 */
	pages := datasize / maxchunksize
	initialchunk := nats.NewMsg(msg.Subject)
	initialchunk.Header = copyHeader(msg)
	initialchunk.Header.Set(msgHeaderChunk, fmt.Sprintf("%d", pages))
	// Copy only the max chunk size from the original message into this first chunk
	initialchunk.Data = msg.Data[:maxchunksize]

	rns.logger.Debug("Chunking Initial Reply Message %s (%s)- pages %d, len %d First Chunk %d", initialchunk.Header.Get(msgHeaderID), initialchunk.Header, pages, len(msg.Data), len(initialchunk.Data))
	chunkchannelmsg, err := rns.Conn.RequestMsgWithContext(ctx, initialchunk)
	if err != nil {
		return errors.Wrap(err, "ChunkSendReplyMsgWithContext")
	}
	/* Reply Message just has a header with the subject we send the rest of the chunks to */
	chunkid := chunkchannelmsg.Header.Get(msgHeaderChunkSubject)
	if chunkid == "" {
		return errors.New("Chunked Reply Response didn't include ChunkID")
	}
	var chunksubject string
	/* The subject we reply to for subsequent chunks might be
	     * coming from another Account, so check the Nats-Request-Info header (set by the NATS server)
		 * to see, and if so, alter the subject we send to chunks to
	*/
	if nri, ok := getNRI(replyto); ok {
		chunksubject = fmt.Sprintf("chunk.%s.send.%s", nri.Acc, chunkid)
	} else {
		chunksubject = fmt.Sprintf("chunk.send.%s", chunkid)
	}
	rns.logger.Trace("Chunk Reply Subject %s", chunksubject)
	/* now, start sending each remaining chunk to the subject, and wait for a reply acknowledging
	 * its reciept.
	 */
	for i := 1; i <= pages; i++ {
		chunkmsg := nats.NewMsg(chunksubject)
		chunkmsg.Header = copyHeader(msg)
		chunkmsg.Header.Set(msgHeaderChunkSeq, fmt.Sprintf("%d", i))
		start := maxchunksize * i
		end := maxchunksize * (i + 1)
		/* make sure we don't overrun our slice */
		if end > len(msg.Data) {
			end = len(msg.Data)
		}
		chunkmsg.Data = msg.Data[start:end]
		rns.logger.Debug("Sending Reply Chunk %s - Page: %d of %d (%d:%d)", chunkmsg.Header.Get(msgHeaderID), i, pages, start, end)
		var chunkack *nats.Msg

		/* If this chunk is not the last chunk, we expect a reply
		 * acknowledging the reciept
		 */
		if i < pages {
			rns.logger.Trace("Sending Chunk to %s", chunkmsg.Subject)
			chunkack, err = rns.Conn.RequestMsgWithContext(ctx, chunkmsg)
			if err != nil {
				return errors.Wrap(err, "ChunkSendReplyMsgWithContext")
			}
			/* XXX TODO: Check Success */
			rns.logger.Trace("Chunk Ack Reply: %s %s - Page %d", chunkack.Header.Get(msgHeaderID), chunkack.Header, i)
		} else {
			/* if its the last chunk, then just send as we wont get a Ack from the
			 * reciever
			 */
			err := rns.Conn.PublishMsg(chunkmsg)
			if err != nil {
				return errors.Wrap(err, "ChunkSendReplyMsgWithContext")
			}
		}

		/* once we have sent everything, return */
		if i == pages {
			return nil
		}
	}
	/* Sending the Chunks failed for some reason. */
	return errors.New("Failed")
}

//ChunkSendRequestMsgWithContext send a message and expect a reply back from the Reciever
//ctx - The Context to use
//conn - the Nats Client Connection
//msg - the message to send
//log - Custom Logger
func (rns *ResticNatsClient) ChunkSendRequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error) {
	if len(msg.Header.Get(msgHeaderID)) == 0 {
		return nil, errors.New("MessageID Not Set")
	}

	//Get the max payload size and use 95% as our limit (to account for any additioanl Meta data)
	var maxchunksize int = int(0.95 * float32(rns.Conn.MaxPayload()))
	datasize := len(msg.Data)
	rns.logger.Debug("ChunkSendRequestMsgWithContext: MsgID %s - Headers %s Size: %d", msg.Header.Get(msgHeaderID), msg.Header, len(msg.Data))

	// If the data is less then our max payload size, just send it without chunking
	if len(msg.Data) < maxchunksize {
		rns.logger.Trace("Short SendRequest MsgID %s - %s Size: %d", msg.Header.Get(msgHeaderID), msg.Header, len(msg.Data))

		reply, err := rns.Conn.RequestMsgWithContext(ctx, msg)
		if err != nil {
			return nil, errors.Wrap(err, "Short Message Send Failure")
		}
		rns.logger.Trace("Short ReplyRequest MsgID %s Headers %s Size: %d", reply.Header.Get(msgHeaderID), reply.Header, len(reply.Data))
		//The reply that came back in may be chunked so parse it and return the final respose
		return rns.ChunkReadRequestMsgWithContext(ctx, reply)
	}

	/* We need to Split the Data into Chunks
	 * The first Chunk will be sent to the replyto Subject and include a Header
	 * indicating this is a chunked message.
	 */
	pages := datasize / maxchunksize
	initialchunk := nats.NewMsg(msg.Subject)
	initialchunk.Header = copyHeader(msg)
	initialchunk.Header.Set(msgHeaderChunk, fmt.Sprintf("%d", pages))

	initialchunk.Data = msg.Data[:maxchunksize]
	rns.logger.Debug("Chunking Send Request MsgID %s - %s- pages %d, len %d First Chunk %d", initialchunk.Header.Get(msgHeaderID), initialchunk.Header, pages, len(msg.Data), len(initialchunk.Data))
	chunkchannelmsg, err := rns.Conn.RequestMsgWithContext(ctx, initialchunk)
	if err != nil {
		return nil, errors.Wrap(err, "chunkRequestMsgWithContext")
	}
	/* Reply Message just has a header with the subject we send the rest of the chunks to */
	chunkid := chunkchannelmsg.Header.Get(msgHeaderChunkSubject)
	if chunkid == "" {
		return nil, errors.New("Chunked Reply Response didn't include ChunkID")
	}
	/* The subject we reply to for subsequent chunks might be
	 * coming from another Account, so check the Nats-Request-Info header (set by the NATS server)
	 * to see, and if so, alter the subject we send to chunks to
	 */
	var chunksubject string
	if nri, ok := getNRI(chunkchannelmsg); ok {
		chunksubject = fmt.Sprintf("chunk.%s.send.%s", nri.Acc, chunkid)
	} else {
		chunksubject = fmt.Sprintf("chunk.send.%s", chunkid)
	}
	/* now send each Chunk */
	for i := 1; i <= pages; i++ {
		chunkmsg := nats.NewMsg(chunksubject)
		chunkmsg.Header = copyHeader(msg)
		chunkmsg.Header.Set(msgHeaderChunkSeq, fmt.Sprintf("%d", i))
		start := maxchunksize * i
		end := maxchunksize * (i + 1)
		/* make sure we don't overrun our slice */
		if end > len(msg.Data) {
			end = len(msg.Data)
		}
		chunkmsg.Data = msg.Data[start:end]
		rns.logger.Debug("Sending Request Chunk %s %s to %s- Page: %d (%d:%d)", chunkmsg.Header.Get(msgHeaderID), chunkmsg.Header, chunkmsg.Subject, i, start, end)
		var chunkackorreply *nats.Msg
		chunkackorreply, err = rns.Conn.RequestMsgWithContext(ctx, chunkmsg)
		if err != nil {
			return nil, errors.Wrap(err, "Chunk Send")
		}
		rns.logger.Trace("got Result %s - %s", chunkmsg.Header.Get(msgHeaderID), chunkmsg.Header)
		/* only the last Chunk Reply will contain the actual Response from the other side
		 * (the other messages were just acks for each Chunk)
		 */
		if i == pages {
			rns.logger.Debug("SendRequest Chunk Reply: MsgID %s Headers %s Size: %d", chunkackorreply.Header.Get(msgHeaderID), chunkackorreply.Header, len(chunkackorreply.Data))
			//The reply might be chunked, so read it and return the final reply
			return rns.ChunkReadRequestMsgWithContext(ctx, chunkackorreply)
		}
	}
	// Chunking Failed for some reason. Die...
	return nil, errors.New("Failed")
}

/* ChunkReadRequestMsgWithContext - Read a message from the Nats Client and if its chunked,
 * get the remaining chunks and reconstruct the message
 * ctx - Context
 * msg - The message we got
 * returns the actual reconstructed message, or a error
 */
func (rns *ResticNatsClient) ChunkReadRequestMsgWithContext(ctx context.Context, msg *nats.Msg) (*nats.Msg, error) {
	if len(msg.Header.Get(msgHeaderID)) == 0 {
		return nil, errors.New("MessageID Not Set")
	}
	rns.logger.Debug("ChunkReadRequestMsgWithContext: MsgID %s - Headers %s Size: %d", msg.Header.Get(msgHeaderID), msg.Header, len(msg.Data))
	chunked := msg.Header.Get(msgHeaderChunk)
	/* if we have the Chunked Header then this message is chunks
	 * so we need to reconstruct it */
	if chunked != "" {
		/* how many Chunks are needed? */
		pages, err := strconv.Atoi(chunked)
		if err != nil {
			return nil, errors.Wrap(err, "Couldn't get Chunk Page Count")
		}
		rns.logger.Debug("Chunked Message Recieved: %s - %s - %d pages", msg.Header.Get(msgHeaderID), msg.Header, pages)
		/* create a random subject to recieve the rest of the chunks from */
		chunktransfer := randStringBytesMaskImprSrcSB(16)
		var chunktransfersubject string
		/* if the message come from another account, we need to read the Nats-Request-Info to get that
		* account and create the subject appropriately */
		if nri, ok := getNRI(msg); ok {
			chunktransfersubject = fmt.Sprintf("chunk.%s.recieve.%s", nri.Acc, chunktransfer)
		} else {
			chunktransfersubject = fmt.Sprintf("chunk.recieve.%s", chunktransfer)
		}
		/* Subscribe to that Subject with a buffered Channel */
		chunkchan := make(chan *nats.Msg, 10)
		sub, err := rns.Conn.QueueSubscribeSyncWithChan(chunktransfersubject, chunktransfer, chunkchan)
		if err != nil {
			return nil, errors.Wrap(err, "Couldn't Subscribe to Chunk Channel")
		}
		/* increase our limits just in case (helps with Slow Consumer issues*/
		sub.SetPendingLimits(1000, 64*1024*1024)
		rns.logger.Trace("Subscription: %+v", sub)
		/* make sure we unsubscribe and close our channels when done */
		defer sub.Unsubscribe()
		defer close(chunkchan)

		/* send back the Channel we will recieve the Chunks on to the sender.
		 * we send back to the original subject */
		chunksubmsg := nats.NewMsg(msg.Reply)
		chunksubmsg.Header = copyHeader(msg)
		chunksubmsg.Header.Add(msgHeaderChunkSubject, chunktransfer)
		if err := msg.RespondMsg(chunksubmsg); err != nil {
			return nil, errors.Wrap(err, "Respond to initial Chunk")
		}
		/* now start recieving our chunks from the sender on the Subject we sent over */
		for i := 1; i <= pages; i++ {
			rns.logger.Debug("Pending MsgID %s Chunk %d of %d on %s", chunksubmsg.Header.Get(msgHeaderID), i, pages, chunktransfersubject)
			select {
			case chunk := <-chunkchan:
				/* got another chunk from the Sending */
				seq, _ := strconv.Atoi(chunk.Header.Get(msgHeaderChunkSeq))
				rns.logger.Debug("Got MsgID %s - %s Chunk %d %d", chunk.Header.Get(msgHeaderID), chunk.Header, seq, i)
				/* this chunk contains Data, Append it to our original message */
				msg.Data = append(msg.Data, chunk.Data...)
				if i < pages {
					/* Everything but the last chunk, we need to Ack back to to the sender */
					ackChunk := nats.NewMsg(chunk.Subject)
					ackChunk.Header = copyHeader(chunk)
					rns.logger.Trace("sending ack %d %d", i, pages)
					err := chunk.RespondMsg(ackChunk)
					if err != nil {
						return nil, errors.Wrap(err, "Chunk Reply Error")
					}
				} else {
					/* Last chunk doesn't need a Ack */
					rns.logger.Trace("Chunked Messages.... %d - %d", i, pages)
					msg.Reply = chunk.Reply
				}
			case <-ctx.Done():
				rns.logger.Debug("Context Canceled")
				return nil, context.DeadlineExceeded
			}
		}
		rns.logger.Debug("Chunked Messages Done - %s - %s Final Size %d", msg.Header.Get(msgHeaderID), msg.Header, len(msg.Data))
	}
	/* return the final message back */
	return msg, nil
}
