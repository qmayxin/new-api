package zhipu_4v

import (
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	channelconstant "github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/claude"
	"github.com/QuantumNous/new-api/relay/channel/openai"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/types"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
)

type Adaptor struct {
}

func (a *Adaptor) ConvertGeminiRequest(*gin.Context, *relaycommon.RelayInfo, *dto.GeminiChatRequest) (any, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, req *dto.ClaudeRequest) (any, error) {
	return req, nil
}

func (a *Adaptor) ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error) {
	//TODO implement me
	return nil, errors.New("not implemented")
}

func (a *Adaptor) ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) Init(info *relaycommon.RelayInfo) {
}

func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	if baseURL == "" {
		baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeZhipu_v4]
	}
	specialPlan, hasSpecialPlan := channelconstant.ChannelSpecialBases[baseURL]

	switch info.RelayFormat {
	case types.RelayFormatClaude:
		if hasSpecialPlan && specialPlan.ClaudeBaseURL != "" {
			return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
		}
		return fmt.Sprintf("%s/api/anthropic/v1/messages", baseURL), nil
	case types.RelayFormatOpenAIResponses, types.RelayFormatOpenAIResponsesCompaction:
		// 将 Responses 格式转换为 Chat Completions 格式
		if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
			return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
		}
		return fmt.Sprintf("%s/api/paas/v4/chat/completions", baseURL), nil
	default:
		switch info.RelayMode {
		case relayconstant.RelayModeEmbeddings:
			if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
				return fmt.Sprintf("%s/embeddings", specialPlan.OpenAIBaseURL), nil
			}
			return fmt.Sprintf("%s/api/paas/v4/embeddings", baseURL), nil
		case relayconstant.RelayModeImagesGenerations:
			if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
				return fmt.Sprintf("%s/images/generations", specialPlan.OpenAIBaseURL), nil
			}
			return fmt.Sprintf("%s/api/paas/v4/images/generations", baseURL), nil
		default:
			if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
				return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
			}
			return fmt.Sprintf("%s/api/paas/v4/chat/completions", baseURL), nil
		}
	}
}

func (a *Adaptor) SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error {
	channel.SetupApiRequestHeader(info, c, req)
	req.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

func (a *Adaptor) ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error) {
	if request == nil {
		return nil, errors.New("request is nil")
	}
	if lo.FromPtrOr(request.TopP, 0) >= 1 {
		request.TopP = lo.ToPtr(0.99)
	}
	return requestOpenAI2Zhipu(*request), nil
}

func (a *Adaptor) ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error) {
	return nil, nil
}

func (a *Adaptor) ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error) {
	return request, nil
}

func (a *Adaptor) ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error) {
	// 将 OpenAI Responses API 格式转换为 Chat Completions 格式
	// 然后交给 requestOpenAI2Zhipu 处理

	// 调试日志
	if common.DebugEnabled {
		inputStr := string(request.Input)
		println("[DEBUG] ConvertOpenAIResponsesRequest - Input:", inputStr)
	}

	messages := make([]dto.Message, 0)

	// 解析 Instructions 作为 system message
	if request.Instructions != nil {
		var instructions string
		if err := common.Unmarshal(request.Instructions, &instructions); err == nil && instructions != "" {
			messages = append(messages, dto.Message{
				Role:    "system",
				Content: instructions,
			})
		}
	}

	// 解析 Input (直接解析为 Input 数组，包含 Role 信息)
	if request.Input != nil {
		var inputs []dto.Input
		if err := common.Unmarshal(request.Input, &inputs); err == nil {
			for _, input := range inputs {
				msg := dto.Message{}

				// 解析 Content
				var content string
				var mediaContent []dto.MediaContent

				switch common.GetJsonType(input.Content) {
				case "string":
					_ = common.Unmarshal(input.Content, &content)
				case "array":
					_ = common.Unmarshal(input.Content, &mediaContent)
				}

				switch input.Type {
				case "input_text":
					msg.Role = input.Role
					if msg.Role == "" {
						msg.Role = "user"
					}
					msg.Content = content

				case "input_image":
					msg.Role = input.Role
					if msg.Role == "" {
						msg.Role = "user"
					}
					// 从 mediaContent 中提取图片
					for _, m := range mediaContent {
						if m.Type == dto.ContentTypeImageURL && m.ImageUrl != nil {
							msg.SetMediaContent([]dto.MediaContent{m})
							break
						}
					}
					if msg.Content == nil {
						msg.Content = content
					}

				case "function_call", "function_call_output":
					// 单独处理 function_call，由 ParseContent 处理
					continue

				default:
					// 其他类型，尝试作为文本处理
					if content != "" {
						msg.Role = input.Role
						if msg.Role == "" {
							msg.Role = "user"
						}
						msg.Content = content
					}
				}

				if msg.Role != "" {
					// 确保 Content 不为 nil
					if msg.Content == nil {
						msg.Content = ""
					}
					messages = append(messages, msg)
				}
			}
		} else {
			// 如果解析失败，尝试作为纯文本处理
			var text string
			if err := common.Unmarshal(request.Input, &text); err == nil {
				messages = append(messages, dto.Message{
					Role:    "user",
					Content: text,
				})
			}
		}
	}

	// 调试日志
	if common.DebugEnabled {
		if len(messages) == 0 {
			println("[DEBUG] ConvertOpenAIResponsesRequest - messages is EMPTY!")
		} else {
			for i, msg := range messages {
				println(fmt.Sprintf("[DEBUG] Message %d: role=%s, content=%v", i, msg.Role, msg.Content))
			}
		}
	}

	// 构建 GeneralOpenAIRequest
	generalReq := dto.GeneralOpenAIRequest{
		Model:       request.Model,
		Messages:    messages,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stream:      request.Stream,
	}

	// 处理 MaxOutputTokens
	if request.MaxOutputTokens != nil {
		generalReq.MaxTokens = request.MaxOutputTokens
	}

	// 处理 Tools - 过滤掉不支持的工具类型（如 web_search）
	if request.Tools != nil {
		var tools []dto.ToolCallRequest
		if err := common.Unmarshal(request.Tools, &tools); err == nil {
			// 只保留 function 类型的工具，过滤掉 web_search 等不支持的类型
			filteredTools := make([]dto.ToolCallRequest, 0, len(tools))
			for _, tool := range tools {
				if tool.Type == "function" {
					filteredTools = append(filteredTools, tool)
				}
			}
			generalReq.Tools = filteredTools
		}
	}

	// 处理 ToolChoice
	if request.ToolChoice != nil {
		generalReq.ToolChoice = request.ToolChoice
	}

	// 处理 Reasoning
	if request.Reasoning != nil && request.Reasoning.Effort != "" {
		generalReq.ReasoningEffort = request.Reasoning.Effort
	}

	// 修正 TopP (如果 >= 1)
	if lo.FromPtrOr(generalReq.TopP, 0) >= 1 {
		generalReq.TopP = lo.ToPtr(0.99)
	}

	// 调用现有的转换函数
	return requestOpenAI2Zhipu(generalReq), nil
}

func (a *Adaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error) {
	return channel.DoApiRequest(a, c, info, requestBody)
}

func (a *Adaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NewAPIError) {
	switch info.RelayFormat {
	case types.RelayFormatClaude:
		adaptor := claude.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	default:
		if info.RelayMode == relayconstant.RelayModeImagesGenerations {
			return zhipu4vImageHandler(c, resp, info)
		}
		adaptor := openai.Adaptor{}
		return adaptor.DoResponse(c, resp, info)
	}
}

func (a *Adaptor) GetModelList() []string {
	return ModelList
}

func (a *Adaptor) GetChannelName() string {
	return ChannelName
}
