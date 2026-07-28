package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ExtMessage struct {
	tgbotapi.Message
	MessageThreadID int `json:"message_thread_id"`
}

type ExtCallbackQuery struct {
	tgbotapi.CallbackQuery
	Message *ExtMessage `json:"message"`
}

type ExtUpdate struct {
	UpdateID      int               `json:"update_id"`
	Message       *ExtMessage       `json:"message"`
	EditedMessage *ExtMessage       `json:"edited_message"`
	CallbackQuery *ExtCallbackQuery `json:"callback_query"`
}

func sendBotWithThread(bot *tgbotapi.BotAPI, c tgbotapi.Chattable, threadID int) (tgbotapi.Message, error) {
	if bot == nil {
		return tgbotapi.Message{}, fmt.Errorf("bot is nil")
	}

	if threadID == 0 {
		return bot.Send(c)
	}

	switch m := c.(type) {
	case tgbotapi.MessageConfig:
		params := tgbotapi.Params{
			"chat_id":           strconv.FormatInt(m.ChatID, 10),
			"text":              m.Text,
			"message_thread_id": strconv.Itoa(threadID),
		}
		if m.ParseMode != "" {
			params["parse_mode"] = m.ParseMode
		}
		if m.DisableWebPagePreview {
			params["disable_web_page_preview"] = "true"
		}
		if m.ReplyMarkup != nil {
			data, _ := json.Marshal(m.ReplyMarkup)
			params["reply_markup"] = string(data)
		}
		resp, err := bot.MakeRequest("sendMessage", params)
		if err != nil {
			return tgbotapi.Message{}, err
		}
		var message tgbotapi.Message
		err = json.Unmarshal(resp.Result, &message)
		return message, err

	case tgbotapi.ChatActionConfig:
		params := tgbotapi.Params{
			"chat_id":           strconv.FormatInt(m.ChatID, 10),
			"action":            m.Action,
			"message_thread_id": strconv.Itoa(threadID),
		}
		_, err := bot.MakeRequest("sendChatAction", params)
		return tgbotapi.Message{}, err

	case tgbotapi.PhotoConfig:
		params := tgbotapi.Params{
			"chat_id":           strconv.FormatInt(m.ChatID, 10),
			"caption":           m.Caption,
			"message_thread_id": strconv.Itoa(threadID),
		}
		if m.ParseMode != "" {
			params["parse_mode"] = m.ParseMode
		}
		if file, ok := m.File.(tgbotapi.FilePath); ok {
			params["photo"] = string(file)
			resp, err := bot.MakeRequest("sendPhoto", params)
			if err != nil {
				return tgbotapi.Message{}, err
			}
			var message tgbotapi.Message
			err = json.Unmarshal(resp.Result, &message)
			return message, err
		}
		return bot.Send(c)

	case tgbotapi.DocumentConfig:
		params := tgbotapi.Params{
			"chat_id":           strconv.FormatInt(m.ChatID, 10),
			"caption":           m.Caption,
			"message_thread_id": strconv.Itoa(threadID),
		}
		if m.ParseMode != "" {
			params["parse_mode"] = m.ParseMode
		}
		if file, ok := m.File.(tgbotapi.FilePath); ok {
			params["document"] = string(file)
			resp, err := bot.MakeRequest("sendDocument", params)
			if err != nil {
				return tgbotapi.Message{}, err
			}
			var message tgbotapi.Message
			err = json.Unmarshal(resp.Result, &message)
			return message, err
		}
		return bot.Send(c)

	default:
		return bot.Send(c)
	}
}

func getUpdatesChan(ctx context.Context, bot *tgbotapi.BotAPI) <-chan ExtUpdate {
	ch := make(chan ExtUpdate, 100)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	go func() {
		offset := 0
		for {
			select {
			case <-ctx.Done():
				close(ch)
				return
			default:
			}

			u.Offset = offset
			resp, err := bot.Request(u)
			if err != nil {
				log.Printf("[telegram] error fetching updates: %v", err)
				select {
				case <-ctx.Done():
					close(ch)
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}

			var updates []ExtUpdate
			if err := json.Unmarshal(resp.Result, &updates); err != nil {
				log.Printf("[telegram] error unmarshaling updates: %v", err)
				select {
				case <-ctx.Done():
					close(ch)
					return
				case <-time.After(3 * time.Second):
					continue
				}
			}

			for _, update := range updates {
				if update.UpdateID >= offset {
					offset = update.UpdateID + 1
					ch <- update
				}
			}
		}
	}()

	return ch
}
