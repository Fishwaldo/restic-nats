package rns

import (
	"context"
	"io"
	"io/ioutil"
)

func (rns *ResticNatsClient) OpenRepo(ctx context.Context, hostname string) (OpenRepoResult, error) {
	var err error
	op := OpenRepoOp{Bucket: rns.bucket, Client: rns.Conn.Opts.Name, Hostname: hostname}
	msg := NewRNSClientMsg(NatsOpenCmd)

	var result OpenRepoResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return OpenRepoResult{}, err
	}

	if !result.Ok {
		rns.logger.Error("Open Repository Failed: %s", result.Err)
	} else {
		rns.clientid = result.ClientID
	}
	return result, nil
}

func (rns *ResticNatsClient) Stat(ctx context.Context, dir, filename string) (StatResult, error) {
	var err error
	op := StatOp{Directory: dir, Filename: filename}
	msg := NewRNSClientMsg(NatsStatCmd)

	var result StatResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return StatResult{}, err
	}

	if !result.Ok {
		rns.logger.Error("Stat File Failed: %s", result.Err)
	}
	return result, nil
}

func (rns *ResticNatsClient) List(ctx context.Context, dir string, recursive bool) (ListResult, error) {
	var err error
	op := ListOp{BaseDir: dir, Recurse: recursive}
	msg := NewRNSClientMsg(NatsListCmd)

	var result ListResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return ListResult{}, err
	}

	if !result.Ok {
		rns.logger.Error("List Dir Failed: %s", result.Err)
	}
	return result, nil
}

func (rns *ResticNatsClient) Load(ctx context.Context, dir string, filename string, length int, offset int64) (LoadResult, error) {
	var err error
	op := LoadOp{Dir: dir, Name: filename, Length: length, Offset: offset}
	msg := NewRNSClientMsg(NatsLoadCmd)

	var result LoadResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return LoadResult{}, err
	}
	if !result.Ok {
		rns.logger.Error("Load Failed: %s", result.Err)
	}
	return result, nil	
}

func (rns *ResticNatsClient) Save(ctx context.Context, dir string, filename string, rd io.Reader) (SaveResult, error) {
	var err error
	op := SaveOp{Dir: dir, Name: filename}
	op.Data, err = ioutil.ReadAll(rd)
	if err != nil {
		rns.logger.Error("ReadAll Failed: %s", err)
		return SaveResult{}, err
	}
	op.Filesize = len(op.Data) 
	msg := NewRNSClientMsg(NatsSaveCmd)

	var result SaveResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return SaveResult{}, err
	}

	if !result.Ok {
		rns.logger.Error("Save Failed: %s", result.Err)
	}
	return result, nil
}

func (rns *ResticNatsClient) Remove(ctx context.Context, dir string, filename string) (RemoveResult, error) {
	var err error
	op := RemoveOp{Dir: dir, Name: filename}
	msg := NewRNSClientMsg(NatsRemoveCmd)

	var result RemoveResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return RemoveResult{}, err
	}

	if !result.Ok {
		rns.logger.Error("Remove File Failed: %s", result.Err)
	}
	return result, nil
}

func (rns *ResticNatsClient) Mkdir(ctx context.Context, dir string) (MkdirResult, error) {
	var err error
	op := MkdirOp{Dir: dir}
	msg := NewRNSClientMsg(NatsMkdirCmd)

	var result MkdirResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return MkdirResult{}, err
	}

	if !result.Ok {
		rns.logger.Error("Mkdir Failed: %s", result.Err)
	}
	return result, nil

}

func (rns *ResticNatsClient) Close(ctx context.Context) (CloseResult, error) {
	var err error
	op := CloseOp{}
	rns.clientid = ""
	msg := NewRNSClientMsg(NatsCloseCmd)

	var result CloseResult
	if err = rns.sendOperation(ctx, msg, op, &result); err != nil {
		return CloseResult{}, err
	}

	if !result.Ok {
		rns.logger.Error("Close Failed: %s", result.Err)
	}
	return result, nil

}