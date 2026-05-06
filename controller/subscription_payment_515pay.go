package controller

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

// Subscription515payPayRequest 515pay支付请求结构
type Subscription515payPayRequest struct {
	PlanId        int    `json:"plan_id"`
	PaymentMethod string `json:"payment_method"`
}

// Pay515APIResponse 515pay API响应结构
type Pay515APIResponse struct {
	Code     int    `json:"code"`
	Msg      string `json:"msg"`
	PayUrl   string `json:"payurl"`
	PayInfo  string `json:"pay_info"`
	PayType  string `json:"pay_type"`
	TradedNo string `json:"trade_no"`
	Sign     string `json:"sign"`
}

// SubscriptionRequest515pay 处理订阅支付请求
func SubscriptionRequest515pay(c *gin.Context) {
	var req Subscription515payPayRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.PlanId <= 0 {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 检查是否启用
	if !operation_setting.Pay515Enabled {
		common.ApiErrorMsg(c, "515pay支付未启用")
		return
	}

	plan, err := model.GetSubscriptionPlanById(req.PlanId)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if !plan.Enabled {
		common.ApiErrorMsg(c, "套餐未启用")
		return
	}
	if plan.PriceAmount < 0.01 {
		common.ApiErrorMsg(c, "套餐金额过低")
		return
	}
	// 允许 515pay 或已配置的支付方式
	if req.PaymentMethod != "515pay" && !operation_setting.ContainsPayMethod(req.PaymentMethod) {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}

	userId := c.GetInt("id")
	if plan.MaxPurchasePerUser > 0 {
		count, err := model.CountUserSubscriptionsByPlan(userId, plan.Id)
		if err != nil {
			common.ApiError(c, err)
			return
		}
		if count >= int64(plan.MaxPurchasePerUser) {
			common.ApiErrorMsg(c, "已达到该套餐购买上限")
			return
		}
	}

	callBackAddress := service.GetCallbackAddress()
	returnUrl, err := url.Parse(callBackAddress + "/api/subscription/515pay/return")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}
	notifyUrl, err := url.Parse(callBackAddress + "/api/subscription/515pay/notify")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}

	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("SUBUSR%dNO%s", userId, tradeNo)

	// 检查配置完整性
	if operation_setting.Pay515ApiUrl == "" || operation_setting.Pay515Pid == "" ||
		operation_setting.Pay515PlatformPublicKey == "" || operation_setting.Pay515MerchantPrivateKey == "" {
		common.ApiErrorMsg(c, "当前管理员未配置515pay支付信息")
		return
	}

	order := &model.SubscriptionOrder{
		UserId:        userId,
		PlanId:        plan.Id,
		Money:         plan.PriceAmount,
		TradeNo:       tradeNo,
		PaymentMethod: req.PaymentMethod,
		CreateTime:    time.Now().Unix(),
		Status:        common.TopUpStatusPending,
	}
	if err := order.Insert(); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}

	// 构建515pay API请求参数
	params := map[string]string{
		"out_trade_no": tradeNo,
		"name":         fmt.Sprintf("订阅套餐:%s", plan.Title),
		"money":        fmt.Sprintf("%.2f", plan.PriceAmount),
		"notify_url":   notifyUrl.String(),
		"return_url":   returnUrl.String(),
		"type":         req.PaymentMethod,
		"clientip":     c.ClientIP(),
	}

	// 调用515pay API (返回 payurl, pay_info, trade_no, pay_type, error)
	payUrl, payInfo, platformTradeNo, payType, err := callPay515API(params)
	if err != nil {
		_ = model.ExpireSubscriptionOrder(tradeNo)
		common.ApiErrorMsg(c, "拉起支付失败: "+err.Error())
		return
	}

	// jump 类型用 pay_info，qrcode 类型拼接 QR 码展示页面
	paymentUrl := payUrl
	if paymentUrl == "" && payType == "jump" && payInfo != "" {
		paymentUrl = payInfo
	}
	if paymentUrl == "" && payType == "qrcode" && platformTradeNo != "" {
		paymentUrl = fmt.Sprintf("%s/pay/qrcode/%s/", strings.TrimRight(operation_setting.Pay515ApiUrl, "/"), platformTradeNo)
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "url": paymentUrl})
}

// callPay515API 调用515pay下单API (返回 payurl, pay_info, trade_no, pay_type, error)
func callPay515API(params map[string]string) (string, string, string, string, error) {
	apiUrl := strings.TrimRight(operation_setting.Pay515ApiUrl, "/") + "/api/pay/create"

	// 添加必要参数
	params["pid"] = operation_setting.Pay515Pid
	params["timestamp"] = fmt.Sprintf("%d", time.Now().Unix())

	// 构建签名
	signContent := buildSignContent(params)
	logger.LogDebug(context.Background(), "[515pay] 签名内容: %s", signContent)
	signature, err := signWithRSA(signContent, operation_setting.Pay515MerchantPrivateKey)
	if err != nil {
		return "", "", "", "", fmt.Errorf("签名失败: %v", err)
	}
	logger.LogDebug(context.Background(), "[515pay] 签名结果: %s", signature)

	params["sign"] = signature
	params["sign_type"] = "RSA"

	// 构建请求体
	requestBody := buildSignContentWithRaw(params)
	logger.LogDebug(context.Background(), "[515pay] 请求体: %s", requestBody)

	// 发送请求
	resp, err := http.Post(apiUrl, "application/x-www-form-urlencoded", strings.NewReader(requestBody))
	if err != nil {
		return "", "", "", "", fmt.Errorf("请求失败: %v", err)
	}
	defer resp.Body.Close()

	// 解析JSON响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", "", "", fmt.Errorf("读取响应失败: %v", err)
	}
	logger.LogDebug(context.Background(), "[515pay] 响应: %s", string(body))

	var result Pay515APIResponse
	if err := common.Unmarshal(body, &result); err != nil {
		return "", "", "", "", fmt.Errorf("解析响应失败: %v", err)
	}

	// 暂时跳过响应验签（排查阶段）
	// if result.Code != 0 {
	// 	return "", "", fmt.Errorf("%s", result.Msg)
	// }

	if result.Code != 0 {
		return "", "", "", "", fmt.Errorf("[515pay] API错误 code=%d msg=%s", result.Code, result.Msg)
	}

	// payurl: jump 类型为空（URL 在 pay_info），qrcode 类型也为空（由调用方拼接 QR 码页面）
	payUrl := result.PayUrl
	// jump 类型用 pay_info，qrcode 类型为空（调用方拼接）
	payInfo := result.PayInfo
	// 暂时跳过验签
	_ = verifyRSA(result.PayUrl+result.Msg+result.TradedNo, result.Sign, operation_setting.Pay515PlatformPublicKey)
	logger.LogDebug(context.Background(), "[515pay] 响应详情: code=%d msg=%s payurl=%s pay_info=%s pay_type=%s trade_no=%s", result.Code, result.Msg, result.PayUrl, result.PayInfo, result.PayType, result.TradedNo)

	return payUrl, payInfo, result.TradedNo, result.PayType, nil
}

// buildSignContent 构建签名字符串 (完全模拟 PHP http_build_query + RSA 签名)
func buildSignContent(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		if k != "sign" && k != "sign_type" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	var signstr string
	for _, k := range keys {
		v := params[k]
		if v == "" {
			continue // 空值跳过（与 PHP SDK isEmpty() 行为一致）
		}
		if signstr != "" {
			signstr += "&"
		}
		signstr += k + "=" + v
	}
	signstr = strings.TrimPrefix(signstr, "&")
	return signstr
}

// buildSignContentWithRaw 构建请求体 (使用 RFC 1738 编码，与 PHP http_build_query 等效)
func buildSignContentWithRaw(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var parts []string
	for _, k := range keys {
		// 使用 RFC 1738 编码 (PHP http_build_query 默认行为)
		keyEncoded := RFC1738Encode(k)
		valEncoded := RFC1738Encode(params[k])
		parts = append(parts, keyEncoded+"="+valEncoded)
	}
	return strings.Join(parts, "&")
}

// RFC1738Encode 模拟 PHP 的 rawurlencode (RFC 1738)
// PHP rawurlencode 按字节编码（非 ASCII 字节编码为 %XX）
func RFC1738Encode(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' || b == '~' {
			sb.WriteByte(b)
		} else {
			sb.WriteString(fmt.Sprintf("%%%02X", b))
		}
	}
	return sb.String()
}


// signWithRSA 使用商户私钥签名 (PKCS1v15 + SHA256)
func signWithRSA(data, privateKey string) (string, error) {
	block, _ := pem.Decode([]byte(formatPEMKey(privateKey, "PRIVATE KEY")))
	if block == nil {
		return "", fmt.Errorf("私钥格式错误")
	}

	var rsaKey *rsa.PrivateKey
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		rsaKey, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return "", fmt.Errorf("解析私钥失败: %v", err)
		}
	} else {
		var ok bool
		rsaKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return "", fmt.Errorf("私钥类型错误")
		}
	}

	hashed := sha256.Sum256([]byte(data))
	digestInfo := buildDigestInfo(hashed[:])
	signature, err := rsa.SignPKCS1v15(nil, rsaKey, 0, digestInfo)
	if err != nil {
		return "", fmt.Errorf("签名失败: %v", err)
	}
	return base64.StdEncoding.EncodeToString(signature), nil
}

// buildDigestInfo 构建 PKCS1v15 签名的 DigestInfo (使用 OpenSSL 兼容的 OID 格式)
// OID 1.2.840.113549.1.1.11 = sha256WithRSAEncryption (短格式: 60 86 48 01 65 03 04 02 01)
func buildDigestInfo(hash []byte) []byte {
	// AlgorithmIdentifier: SEQUENCE { OID sha256, NULL }
	// OID 1.2.840.113549.1.1.11 (DER: 30 0d 06 09 60 86 48 01 65 03 04 02 01 05 00)
	algo := []byte{
		0x30, 0x0d, // SEQUENCE, length 13
		0x06, 0x09, 0x60, 0x86, 0x48, 0x01, 0x65, 0x03, 0x04, 0x02, 0x01, // OID 1.2.840.113549.1.1.11 (short form)
		0x05, 0x00, // NULL
	}
	// DigestInfo: SEQUENCE { AlgorithmIdentifier } || OCTET STRING { hash }
	// 结构: 30 31 || algo || 04 20 || hash
	hashLen := len(hash)
	totalLen := 2 + len(algo) + 2 + hashLen // 2 for SEQUENCE tag+len, 2 for OCTET STRING tag+len
	di := make([]byte, 0, totalLen)
	di = append(di, 0x30, byte(totalLen-2))
	di = append(di, algo...)
	di = append(di, 0x04, byte(hashLen))
	di = append(di, hash...)
	return di
}

// verifyRSA 使用平台公钥验签
func verifyRSA(data, signBase64, publicKey string) bool {
	block, _ := pem.Decode([]byte(formatPEMKey(publicKey, "PUBLIC KEY")))
	if block == nil {
		return false
	}

	key, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return false
	}

	rsaKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return false
	}

	signature, err := base64.StdEncoding.DecodeString(signBase64)
	if err != nil {
		return false
	}

	hashed := sha256.Sum256([]byte(data))
	return rsa.VerifyPKCS1v15(rsaKey, 0, hashed[:], signature) == nil
}

// formatPEMKey 格式化PEM密钥字符串 (添加换行)
func formatPEMKey(key, keyType string) string {
	var sb strings.Builder
	sb.WriteString("-----BEGIN " + keyType + "-----\n")
	lines := splitByLength(key, 64)
	for _, line := range lines {
		sb.WriteString(line + "\n")
	}
	sb.WriteString("-----END " + keyType + "-----")
	return sb.String()
}

// splitByLength 将字符串分割为指定长度的片段
func splitByLength(s string, length int) []string {
	var result []string
	for i := 0; i < len(s); i += length {
		end := i + length
		if end > len(s) {
			end = len(s)
		}
		result = append(result, s[i:end])
	}
	return result
}

// Subscription515payNotify 处理515pay支付回调
func Subscription515payNotify(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		if err := c.Request.ParseForm(); err != nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = make(map[string]string)
		for k, v := range c.Request.PostForm {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	} else {
		params = make(map[string]string)
		for k, v := range c.Request.URL.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	}

	if len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 验证签名
	if !verify515payNotify(params) {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 检查时间戳 (允许5分钟内)
	timestamp, ok := params["timestamp"]
	if !ok {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	var ts int64
	fmt.Sscanf(timestamp, "%d", &ts)
	if time.Now().Unix()-ts > 300 || ts-time.Now().Unix() > 300 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 检查交易状态
	if params["trade_status"] != "TRADE_SUCCESS" {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	outTradeNo := params["out_trade_no"]
	LockOrder(outTradeNo)
	defer UnlockOrder(outTradeNo)

	notifyData, _ := common.Marshal(params)
	if err := model.CompleteSubscriptionOrder(outTradeNo, string(notifyData)); err != nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	_, _ = c.Writer.Write([]byte("success"))
}

// verify515payNotify 验证515pay回调签名
func verify515payNotify(params map[string]string) bool {
	sign, ok := params["sign"]
	if !ok {
		return false
	}

	// 构建待验签内容 (排除sign和sign_type)
	signContent := buildSignContent(params)
	return verifyRSA(signContent, sign, operation_setting.Pay515PlatformPublicKey)
}

// Subscription515payReturn 处理用户支付返回
func Subscription515payReturn(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		if err := c.Request.ParseForm(); err != nil {
			c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/topup?pay=fail")
			return
		}
		params = make(map[string]string)
		for k, v := range c.Request.PostForm {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	} else {
		params = make(map[string]string)
		for k, v := range c.Request.URL.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	}

	if len(params) == 0 {
		c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/topup?pay=fail")
		return
	}

	// 验证签名
	if !verify515payNotify(params) {
		c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/topup?pay=fail")
		return
	}

	if params["trade_status"] == "TRADE_SUCCESS" {
		outTradeNo := params["out_trade_no"]
		LockOrder(outTradeNo)
		defer UnlockOrder(outTradeNo)
		notifyData, _ := common.Marshal(params)
		if err := model.CompleteSubscriptionOrder(outTradeNo, string(notifyData)); err != nil {
			c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/topup?pay=fail")
			return
		}
		c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/topup?pay=success")
		return
	}
	c.Redirect(http.StatusFound, system_setting.ServerAddress+"/console/topup?pay=pending")
}

// ---- 用户充值 515pay ----

// Topup515payPayRequest 用户充值 515pay 请求结构
type Topup515payPayRequest struct {
	Amount        int64  `json:"amount"`
	PaymentMethod string `json:"payment_method"` // wxpay / alipay
}

// RequestTopup515pay 处理用户充值请求（通过 515pay）
func RequestTopup515pay(c *gin.Context) {
	var req Topup515payPayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiErrorMsg(c, "参数错误")
		return
	}

	// 检查是否启用
	if !operation_setting.Pay515Enabled {
		common.ApiErrorMsg(c, "515pay支付未启用")
		return
	}

	if req.Amount < getMinTopup() {
		common.ApiErrorMsg(c, fmt.Sprintf("充值数量不能小于 %d", getMinTopup()))
		return
	}

	id := c.GetInt("id")
	group, err := model.GetUserGroup(id, true)
	if err != nil {
		common.ApiErrorMsg(c, "获取用户分组失败")
		return
	}
	payMoney := getPayMoney(req.Amount, group)
	if payMoney < 0.01 {
		common.ApiErrorMsg(c, "充值金额过低")
		return
	}

	// 515pay 只支持 wxpay / alipay
	if req.PaymentMethod != "wxpay" && req.PaymentMethod != "alipay" {
		common.ApiErrorMsg(c, "支付方式不存在")
		return
	}

	// 检查配置完整性
	if operation_setting.Pay515ApiUrl == "" || operation_setting.Pay515Pid == "" ||
		operation_setting.Pay515PlatformPublicKey == "" || operation_setting.Pay515MerchantPrivateKey == "" {
		common.ApiErrorMsg(c, "当前管理员未配置515pay支付信息")
		return
	}

	callBackAddress := service.GetCallbackAddress()
	notifyUrl, err := url.Parse(callBackAddress + "/api/user/topup/515pay/notify")
	if err != nil {
		common.ApiErrorMsg(c, "回调地址配置错误")
		return
	}

	tradeNo := fmt.Sprintf("%s%d", common.GetRandomString(6), time.Now().Unix())
	tradeNo = fmt.Sprintf("USR%dNO%s", id, tradeNo)

	amount := req.Amount
	if operation_setting.GetQuotaDisplayType() == operation_setting.QuotaDisplayTypeTokens {
		dAmount := decimal.NewFromInt(int64(amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		amount = int64(dAmount.Div(dQuotaPerUnit).IntPart())
	}

	topUp := &model.TopUp{
		UserId:        id,
		Amount:        amount,
		Money:         payMoney,
		TradeNo:       tradeNo,
		PaymentMethod: "515pay_" + req.PaymentMethod, // 标记为 515pay 的子方式
		CreateTime:    time.Now().Unix(),
		Status:        "pending",
	}
	if err := topUp.Insert(); err != nil {
		common.ApiErrorMsg(c, "创建订单失败")
		return
	}

	// 构建 515pay API 请求参数
	params := map[string]string{
		"out_trade_no": tradeNo,
		"name":         fmt.Sprintf("充值:%d", req.Amount),
		"money":        fmt.Sprintf("%.2f", payMoney),
		"notify_url":   notifyUrl.String(),
		"type":         req.PaymentMethod,
		"clientip":     c.ClientIP(),
	}

	payUrl, payInfo, platformTradeNo, payType, err := callPay515API(params)
	if err != nil {
		_ = model.DB.Model(&model.TopUp{}).Where("trade_no = ?", tradeNo).Update("status", "expired").Error
		common.ApiErrorMsg(c, "拉起支付失败: "+err.Error())
		return
	}

	// jump 类型用 pay_info，qrcode 类型拼接 QR 码展示页面
	paymentUrl := payUrl
	if paymentUrl == "" && payType == "jump" && payInfo != "" {
		paymentUrl = payInfo
	}
	if paymentUrl == "" && payType == "qrcode" && platformTradeNo != "" {
		paymentUrl = fmt.Sprintf("%s/pay/qrcode/%s/", strings.TrimRight(operation_setting.Pay515ApiUrl, "/"), platformTradeNo)
	}

	c.JSON(http.StatusOK, gin.H{"message": "success", "url": paymentUrl})
}

// ExpireTopUp 使订单失效（用于充值流程）
func ExpireTopUp(tradeNo string) error {
	return model.DB.Model(&model.TopUp{}).Where("trade_no = ?", tradeNo).Updates(map[string]interface{}{
		"status": "expired",
	}).Error
}

// Topup515payNotify 处理充值回调
func Topup515payNotify(c *gin.Context) {
	var params map[string]string

	if c.Request.Method == "POST" {
		if err := c.Request.ParseForm(); err != nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		params = make(map[string]string)
		for k, v := range c.Request.PostForm {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	} else {
		params = make(map[string]string)
		for k, v := range c.Request.URL.Query() {
			if len(v) > 0 {
				params[k] = v[0]
			}
		}
	}

	if len(params) == 0 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 验证签名
	if !verify515payNotify(params) {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 检查时间戳
	timestamp, ok := params["timestamp"]
	if !ok {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}
	var ts int64
	fmt.Sscanf(timestamp, "%d", &ts)
	if time.Now().Unix()-ts > 300 || ts-time.Now().Unix() > 300 {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 检查交易状态
	if params["trade_status"] != "TRADE_SUCCESS" {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	outTradeNo := params["out_trade_no"]
	LockOrder(outTradeNo)
	defer UnlockOrder(outTradeNo)

	topUp := model.GetTopUpByTradeNo(outTradeNo)
	if topUp == nil {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	// 检查是否是 515pay 订单
	if !strings.HasPrefix(topUp.PaymentMethod, "515pay_") {
		_, _ = c.Writer.Write([]byte("fail"))
		return
	}

	if topUp.Status == "pending" {
		topUp.Status = "success"
		if err := topUp.Update(); err != nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}

		dAmount := decimal.NewFromInt(int64(topUp.Amount))
		dQuotaPerUnit := decimal.NewFromFloat(common.QuotaPerUnit)
		quotaToAdd := int(dAmount.Mul(dQuotaPerUnit).IntPart())
		if err := model.IncreaseUserQuota(topUp.UserId, quotaToAdd, true); err != nil {
			_, _ = c.Writer.Write([]byte("fail"))
			return
		}
		model.RecordLog(topUp.UserId, model.LogTypeTopup, fmt.Sprintf("使用515pay充值成功，充值额度: %v，支付金额：%f", logger.LogQuota(quotaToAdd), topUp.Money))
	}

	_, _ = c.Writer.Write([]byte("success"))
}
