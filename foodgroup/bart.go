package foodgroup

import (
	"context"
	"crypto/md5"
	"errors"
	"fmt"
	"log/slog"

	"github.com/mk6i/retro-aim-server/state"
	"github.com/mk6i/retro-aim-server/wire"
)

// blankGIF is a blank, transparent 50x50p GIF that takes the place of a
// cleared buddy icon.
var blankGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x32, 0x00, 0x32, 0x00, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x32, 0x00, 0x32, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
}

// errKnownIconsOnly indicates that a non-known buddy icon was requested
var errKnownIconsOnly = errors.New("can only satisfy requests for known icons")

func NewBARTService(logger *slog.Logger, bartManager BARTManager, messageRelayer MessageRelayer, feedbagManager FeedbagManager) BARTService {
	return BARTService{
		bartManager:    bartManager,
		feedbagManager: feedbagManager,
		logger:         logger,
		messageRelayer: messageRelayer,
	}
}

type BARTService struct {
	bartManager    BARTManager
	feedbagManager FeedbagManager
	logger         *slog.Logger
	messageRelayer MessageRelayer
}

func (s BARTService) UpsertItem(ctx context.Context, sess *state.Session, inFrame wire.SNACFrame, inBody wire.SNAC_0x10_0x02_BARTUploadQuery) (wire.SNACMessage, error) {
	h := md5.New()
	if _, err := h.Write(inBody.Data); err != nil {
		return wire.SNACMessage{}, err
	}
	hash := h.Sum(nil)

	if err := s.bartManager.BARTUpsert(hash, inBody.Data); err != nil {
		return wire.SNACMessage{}, err
	}

	s.logger.DebugContext(ctx, "successfully uploaded buddy icon", "hash", fmt.Sprintf("%x", hash))

	if err := broadcastArrival(ctx, sess, s.messageRelayer, s.feedbagManager); err != nil {
		return wire.SNACMessage{}, err
	}

	return wire.SNACMessage{
		Frame: wire.SNACFrame{
			FoodGroup: wire.BART,
			SubGroup:  wire.BARTUploadReply,
			RequestID: inFrame.RequestID,
		},
		Body: wire.SNAC_0x10_0x03_BARTUploadReply{
			Code: wire.BARTReplyCodesSuccess,
			ID: wire.BARTID{
				Type: inBody.Type,
				BARTInfo: wire.BARTInfo{
					Flags: wire.BARTFlagsKnown,
					Hash:  hash,
				},
			},
		},
	}, nil
}

func (s BARTService) RetrieveItem(ctx context.Context, sess *state.Session, inFrame wire.SNACFrame, inBody wire.SNAC_0x10_0x04_BARTDownloadQuery) (wire.SNACMessage, error) {
	if inBody.Flags != wire.BARTFlagsKnown {
		return wire.SNACMessage{}, errKnownIconsOnly
	}

	var icon []byte
	if inBody.HasClearIconHash() {
		icon = blankGIF
	} else {
		var err error
		if icon, err = s.bartManager.BARTRetrieve(inBody.Hash); err != nil {
			return wire.SNACMessage{}, err
		}
	}

	// todo... how to reply if requested icon doesn't exist
	return wire.SNACMessage{
		Frame: wire.SNACFrame{
			FoodGroup: wire.BART,
			SubGroup:  wire.BARTDownloadReply,
			RequestID: inFrame.RequestID,
		},
		Body: wire.SNAC_0x10_0x05_BARTDownloadReply{
			ScreenName: inBody.ScreenName,
			BARTID:     inBody.BARTID,
			Data:       icon,
		},
	}, nil
}
