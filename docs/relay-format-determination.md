# RelayFormat 取值确定流程

本文档详细说明了 `RelayFormat` 的取值是如何确定的，以及它在系统中的作用。

## 概述

`RelayFormat` 是一个类型定义，用于标识请求的格式类型，系统根据不同的格式类型选择不同的处理逻辑。

```go
// types/relay_format.go
type RelayFormat string

const (
    RelayFormatOpenAI                    RelayFormat = "openai"
    RelayFormatClaude                                = "claude"
    RelayFormatGemini                                = "gemini"
    RelayFormatOpenAIResponses                       = "openai_responses"
    RelayFormatOpenAIResponsesCompaction             = "openai_responses_compaction"
    RelayFormatOpenAIAudio                           = "openai_audio"
    RelayFormatOpenAIImage                           = "openai_image"
    RelayFormatOpenAIRealtime                        = "openai_realtime"
    RelayFormatRerank                                = "rerank"
    RelayFormatEmbedding                             = "embedding"
    RelayFormatTask                                  = "task"
    RelayFormatMjProxy                               = "mj_proxy"
)
```

## 取值确定流程

### 1. 路由层决定格式 (router/relay-router.go)

`RelayFormat` 完全由**请求路径**决定，路由层根据不同路径直接指定对应的格式：

| 请求路径 | RelayFormat |
|---------|-------------|
| `/v1/messages` | `RelayFormatClaude` |
| `/v1/chat/completions` | `RelayFormatOpenAI` |
| `/v1/completions` | `RelayFormatOpenAI` |
| `/v1/responses` | `RelayFormatOpenAIResponses` |
| `/v1/responses/compact` | `RelayFormatOpenAIResponsesCompaction` |
| `/v1/images/generations` | `RelayFormatOpenAIImage` |
| `/v1/images/edits` | `RelayFormatOpenAIImage` |
| `/v1/edits` | `RelayFormatOpenAIImage` |
| `/v1/embeddings` | `RelayFormatEmbedding` |
| `/v1/audio/transcriptions` | `RelayFormatOpenAIAudio` |
| `/v1/audio/translations` | `RelayFormatOpenAIAudio` |
| `/v1/audio/speech` | `RelayFormatOpenAIAudio` |
| `/v1/rerank` | `RelayFormatRerank` |
| `/v1beta/models/*` | `RelayFormatGemini` |
| `/v1/realtime` | `RelayFormatOpenAIRealtime` |
| `/v1/moderations` | `RelayFormatOpenAI` |

代码示例：

```go
// router/relay-router.go
httpRouter.POST("/messages", func(c *gin.Context) {
    controller.Relay(c, types.RelayFormatClaude)
})

httpRouter.POST("/chat/completions", func(c *gin.Context) {
    controller.Relay(c, types.RelayFormatOpenAI)
})
```

### 2. 控制器层传递格式 (controller/relay.go)

`Relay(c *gin.Context, relayFormat types.RelayFormat)` 函数接收路由传来的格式，并将其传递给 `GenRelayInfo`：

```go
// controller/relay.go
func Relay(c *gin.Context, relayFormat types.RelayFormat) {
    // ...
    relayInfo, err := relaycommon.GenRelayInfo(c, relayFormat, request, ws)
    // ...
}
```

### 3. RelayInfo 初始化 (relay/common/relay_info.go)

`GenRelayInfo` 根据 `relayFormat` 参数调用对应的构造函数：

```go
// relay/common/relay_info.go
func GenRelayInfo(c *gin.Context, relayFormat types.RelayFormat, request dto.Request, ws *websocket.Conn) (*RelayInfo, error) {
    var info *RelayInfo
    var err error
    switch relayFormat {
    case types.RelayFormatOpenAI:
        info = GenRelayInfoOpenAI(c, request)
    case types.RelayFormatClaude:
        info = GenRelayInfoClaude(c, request)
    case types.RelayFormatGemini:
        info = GenRelayInfoGemini(c, request)
    // ... 其他格式
    }
    // ...
}
```

各个构造函数会设置对应的 `RelayFormat`：

```go
func GenRelayInfoClaude(c *gin.Context, request dto.Request) *RelayInfo {
    info := genBaseRelayInfo(c, request)
    info.RelayFormat = types.RelayFormatClaude  // 设置格式
    // ...
    return info
}

func GenRelayInfoOpenAI(c *gin.Context, request dto.Request) *RelayInfo {
    info := genBaseRelayInfo(c, request)
    info.RelayFormat = types.RelayFormatOpenAI  // 设置格式
    return info
}
```

### 4. 在 channel adaptor 中的使用

以 zhipu_4v 为例，`GetRequestURL` 函数根据 `info.RelayFormat` 来决定返回不同的 URL：

```go
// relay/channel/zhipu_4v/adaptor.go
func (a *Adaptor) GetRequestURL(info *relaycommon.RelayInfo) (string, error) {
    baseURL := info.ChannelBaseUrl
    if baseURL == "" {
        baseURL = channelconstant.ChannelBaseURLs[channelconstant.ChannelTypeZhipu_v4]
    }
    specialPlan, hasSpecialPlan := channelconstant.ChannelSpecialBases[baseURL]

    switch info.RelayFormat {
    case types.RelayFormatClaude:
        // 返回 Claude 格式的 URL
        if hasSpecialPlan && specialPlan.ClaudeBaseURL != "" {
            return fmt.Sprintf("%s/v1/messages", specialPlan.ClaudeBaseURL), nil
        }
        return fmt.Sprintf("%s/api/anthropic/v1/messages", baseURL), nil
    default:
        // 默认（OpenAI 格式）返回 OpenAI 格式的 URL
        switch info.RelayMode {
        case relayconstant.RelayModeEmbeddings:
            // ...
        case relayconstant.RelayModeImagesGenerations:
            // ...
        default:
            if hasSpecialPlan && specialPlan.OpenAIBaseURL != "" {
                return fmt.Sprintf("%s/chat/completions", specialPlan.OpenAIBaseURL), nil
            }
            return fmt.Sprintf("%s/api/paas/v4/chat/completions", baseURL), nil
        }
    }
}
```

同样在 `DoResponse` 中：

```go
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
```

## 总结

**`RelayFormat` 的取值是由请求的 URL 路径决定的**，与使用哪个 channel（如 zhipu_4v）无关。同一个 channel 可以根据不同的 `RelayFormat` 处理不同格式的请求，实现多协议适配。

关键要点：
1. 路由层根据路径硬编码指定 `RelayFormat`
2. 格式通过 `RelayInfo` 结构体传递到整个处理链路
3. Channel adaptor 根据 `RelayFormat` 分支处理不同格式的请求和响应
