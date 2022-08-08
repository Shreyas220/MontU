package main

import (
	"fmt"

	"github.com/slack-go/slack"
	"go.uber.org/zap"
)

var api = slack.New("")

//SendSlackMsg is to send msg to slack channel using ChannelID
func SendSlackMsg(Msg string) {
	_, _, err := api.PostMessage(
		"",
		slack.MsgOptionText(Msg, true),
	)
	if err != nil {
		fmt.Println("error while sending slack alert", zap.String("err_msg", err.Error()), zap.String("msg", Msg))
	}
}
