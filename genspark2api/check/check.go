package check

import (
	"genspark2api/common"
	"genspark2api/common/config"
	logger "genspark2api/common/loggger"
	"regexp"
	"strings"

	"github.com/samber/lo"
)

func CheckEnvVariable() {
	logger.SysLog("environment variable checking...")

	if config.GSCookie == "" {
		logger.SysLog("环境变量 GS_COOKIE 未设置：服务仍可启动，但调用文本/图片/视频接口会失败（需要至少包含 session_id=...）")
	}
	if config.YesCaptchaClientKey == "" {
		//logger.SysLog("环境变量 YES_CAPTCHA_CLIENT_KEY 未设置，将无法使用 YesCaptcha 过谷歌验证，导致无法调用文生图模型 \n ClientKey获取地址：https://yescaptcha.com/i/021iAE")
	}
	if config.ModelChatMapStr != "" {
		//pattern := `^([a-zA-Z0-9\-\/]+=([a-zA-Z0-9\-\.]+))(,[a-zA-Z0-9\-\/]+=([a-zA-Z0-9\-\.]+))*`
		pattern := `^([a-zA-Z0-9\-\/\.]+=([a-zA-Z0-9\-\.]+))(,[a-zA-Z0-9\-\/\.]+=([a-zA-Z0-9\-\.]+))*$`

		match, _ := regexp.MatchString(pattern, config.ModelChatMapStr)
		if !match {
			logger.FatalLog("环境变量 MODEL_CHAT_MAP 设置有误")
		} else {
			modelChatMap := make(map[string]string)
			pairs := strings.Split(config.ModelChatMapStr, ",")

			for _, pair := range pairs {
				kv := strings.Split(pair, "=")
				if !lo.Contains(common.DefaultOpenaiModelList, kv[0]) {
					logger.FatalLog("环境变量 MODEL_CHAT_MAP 中 MODEL 有误")
				}
				modelChatMap[kv[0]] = kv[1]
			}

			config.ModelChatMap = modelChatMap

			if config.AutoModelChatMapType == 1 {
				// Historically this project treated MODEL_CHAT_MAP (explicit bindings) as mutually exclusive
				// with AUTO_MODEL_CHAT_MAP_TYPE=1 (auto-learn bindings). In practice they can coexist safely:
				// explicit bindings are always preferred, and auto-learning simply records additional bindings.
				logger.SysLog("环境变量 MODEL_CHAT_MAP 与 AUTO_MODEL_CHAT_MAP_TYPE=1 同时启用：将优先使用 MODEL_CHAT_MAP，且仍会自动学习其它模型的绑定")
			}

		}
	}

	if config.SessionImageChatMapStr != "" {
		//pattern := `^([a-zA-Z0-9\-\/]+=([a-zA-Z0-9\-\.]+))(,[a-zA-Z0-9\-\/]+=([a-zA-Z0-9\-\.]+))*`
		pattern := `^([a-zA-Z0-9\-\/\.]+=([a-zA-Z0-9\-\.]+))(,[a-zA-Z0-9\-\/\.]+=([a-zA-Z0-9\-\.]+))*$`

		match, _ := regexp.MatchString(pattern, config.SessionImageChatMapStr)
		if !match {
			logger.FatalLog("环境变量 SESSION_IMAGE_CHAT_MAP 设置有误")
		} else {
			sessionImageChatMap := make(map[string]string)
			pairs := strings.Split(config.SessionImageChatMapStr, ",")

			for _, pair := range pairs {
				kv := strings.Split(pair, "=")
				sessionImageChatMap["session_id="+kv[0]] = kv[1]
			}

			config.SessionImageChatMap = sessionImageChatMap
		}
	} else {
		//logger.SysLog("环境变量 SESSION_IMAGE_CHAT_MAP 未设置，生图可能会异常")
	}

	logger.SysLog("environment variable check passed.")
}
