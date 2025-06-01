package handler

import (
	"context"
	"log"
	"sort"

	"github.com/gocarina/gocsv"
	"github.com/korotovsky/slack-mcp-server/pkg/provider"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/slack-go/slack"
)

var AllChanTypes = []string{"mpim", "im", "public_channel", "private_channel"}
var PubChanType = []string{"public_channel"}

type Channel struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Topic       string `json:"topic"`
	Purpose     string `json:"purpose"`
	MemberCount int    `json:"memberCount"`
}

type ChannelsHandler struct {
	apiProvider *provider.ApiProvider
}

func NewChannelsHandler(apiProvider *provider.ApiProvider) *ChannelsHandler {
	return &ChannelsHandler{
		apiProvider: apiProvider,
	}
}

func (ch *ChannelsHandler) ChannelsHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sortType := request.GetString("sort", "popularity")
	channelTypes := request.GetStringSlice("channel_types", PubChanType)

	api, err := ch.apiProvider.Provide()
	if err != nil {
		return nil, err
	}

	var channels []slack.Channel

	params := &slack.GetConversationsParameters{
		Types:           channelTypes,
		Limit:           100,
		ExcludeArchived: true,
	}
	var total int
	for i := 1; ; i++ {
		var (
			chans   []slack.Channel
			nextcur string
		)

		chans, nextcur, err = api.GetConversationsContext(ctx, params)

		if nextcur == "" {
			log.Printf("channels fetch complete %v", total)
			break
		}

		params.Cursor = nextcur

		channels = append(channels, chans...)
	}

	var channelList []Channel
	for _, channel := range channels {
		channelList = append(channelList, Channel{
			ID:          channel.ID,
			Name:        "#" + channel.Name,
			Topic:       channel.Topic.Value,
			Purpose:     channel.Purpose.Value,
			MemberCount: channel.NumMembers,
		})
	}

	switch sortType {
	case "popularity":
		sort.Slice(channelList, func(i, j int) bool {
			return channelList[i].MemberCount > channelList[j].MemberCount
		})
	default:
		// pass
	}

	csvBytes, err := gocsv.MarshalBytes(&channelList)
	if err != nil {
		return nil, err
	}

	return mcp.NewToolResultText(string(csvBytes)), nil
}
