package slack

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/Jongseong0111/jarvis/domain"
	"github.com/Jongseong0111/jarvis/pkg/log"
	slackgo "github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

// Client 는 Slack Socket Mode 연결을 관리하고 메시지를 송수신한다.
// domain.MessageSender 를 구현한다.
type Client struct {
	api         *slackgo.Client
	socket      *socketmode.Client
	botID       string
	interaction *InteractionHandler // 버튼 클릭 처리(선택)
}

// SetInteractionHandler 는 버튼(interactive) 처리기를 연결한다.
func (c *Client) SetInteractionHandler(h *InteractionHandler) { c.interaction = h }

// NewClient 는 봇/앱 토큰으로 Socket Mode 클라이언트를 생성한다.
func NewClient(botToken, appToken string) (*Client, error) {
	api := slackgo.New(botToken, slackgo.OptionAppLevelToken(appToken))
	auth, err := api.AuthTest()
	if err != nil {
		return nil, fmt.Errorf("slack 인증 실패: %w", err)
	}
	return &Client{
		api:    api,
		socket: socketmode.New(api),
		botID:  auth.UserID,
	}, nil
}

// Send 는 채널로 메시지를 전송한다. 버튼이 있으면 Block Kit 으로 전송한다.
func (c *Client) Send(ctx context.Context, reply domain.Reply) error {
	var opt slackgo.MsgOption
	if len(reply.Buttons) > 0 {
		opt = slackgo.MsgOptionBlocks(buildBlocks(reply)...)
	} else {
		opt = slackgo.MsgOptionText(reply.Text, false)
	}
	if _, _, err := c.api.PostMessageContext(ctx, reply.ChannelID, opt); err != nil {
		return fmt.Errorf("slack 메시지 전송 실패: %w", err)
	}
	return nil
}

// Run 은 이벤트 루프를 실행한다(ctx 취소까지 블로킹).
func (c *Client) Run(ctx context.Context, handler Handler) error {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-c.socket.Events:
				if !ok {
					return
				}
				switch evt.Type {
				case socketmode.EventTypeEventsAPI:
					eventsAPI, ok := evt.Data.(slackevents.EventsAPIEvent)
					if !ok {
						continue
					}
					// Ack 는 error 를 반환하므로 무시하지 않고 로깅한다.
					if err := c.socket.Ack(*evt.Request); err != nil {
						log.FromContext(ctx).Error("slack ack 실패", "error", err)
					}
					c.dispatch(ctx, eventsAPI, handler)
				case socketmode.EventTypeInteractive:
					callback, ok := evt.Data.(slackgo.InteractionCallback)
					if !ok {
						continue
					}
					if err := c.socket.Ack(*evt.Request); err != nil {
						log.FromContext(ctx).Error("slack ack 실패", "error", err)
					}
					if c.interaction != nil {
						if err := c.interaction.Handle(ctx, callback); err != nil {
							log.FromContext(ctx).Error("버튼 처리 실패", "error", err)
						}
					}
				default:
					continue
				}
			}
		}
	}()
	if err := c.socket.RunContext(ctx); err != nil {
		return fmt.Errorf("socket 실행 종료: %w", err)
	}
	return nil
}

// dispatch 는 Slack 이벤트를 IncomingMessage 로 변환해 handler 에 전달한다.
// app_mention 과 DM(im) 만 처리하고, 봇 자신/서브타입 메시지는 무시한다.
func (c *Client) dispatch(ctx context.Context, event slackevents.EventsAPIEvent, handler Handler) {
	if event.Type != slackevents.CallbackEvent {
		return
	}

	var in domain.IncomingMessage
	switch ev := event.InnerEvent.Data.(type) {
	case *slackevents.AppMentionEvent:
		// 다른 봇이 @jarvis 를 멘션하는 경우 무시한다.
		if ev.BotID != "" {
			return
		}
		in = domain.IncomingMessage{ChannelID: ev.Channel, UserID: ev.User, Text: ev.Text}
		in.Images = c.downloadImages(ctx, ev.Files)
	case *slackevents.MessageEvent:
		if ev.ChannelType != "im" || ev.SubType != "" || ev.BotID != "" || ev.User == c.botID {
			return
		}
		in = domain.IncomingMessage{ChannelID: ev.Channel, UserID: ev.User, Text: ev.Text}
		// MessageEvent 는 Files 필드가 없으므로 Message.Files 에서 가져온다.
		if ev.Message != nil {
			in.Images = c.downloadImages(ctx, ev.Message.Files)
		}
	default:
		return
	}

	if err := handler.Handle(ctx, in); err != nil {
		log.FromContext(ctx).Error("메시지 처리 실패", "error", err)
	}
}

// maxImageBytes 는 다운로드 허용 이미지 최대 크기다(과대 파일 방지).
const maxImageBytes = 10 * 1024 * 1024

// isImageMime 은 image/* MIME 인지 판별한다.
func isImageMime(mime string) bool {
	return strings.HasPrefix(mime, "image/")
}

// downloadImages 는 Slack 첨부 중 이미지들을 봇토큰으로 다운로드한다.
// 개별 실패는 건너뛴다(best-effort) — 전체를 실패시키지 않는다.
func (c *Client) downloadImages(ctx context.Context, files []slackgo.File) []domain.Image {
	var out []domain.Image
	for _, f := range files {
		if !isImageMime(f.Mimetype) {
			continue
		}
		url := f.URLPrivateDownload
		if url == "" {
			url = f.URLPrivate
		}
		if url == "" {
			continue
		}
		var buf bytes.Buffer
		if err := c.api.GetFileContext(ctx, url, &buf); err != nil {
			log.FromContext(ctx).Error("이미지 다운로드 실패", "error", err, "url", url)
			continue
		}
		if buf.Len() == 0 || buf.Len() > maxImageBytes {
			log.FromContext(ctx).Error("이미지 크기 부적합", "bytes", buf.Len())
			continue
		}
		out = append(out, domain.Image{Data: buf.Bytes(), MIME: f.Mimetype})
	}
	return out
}
