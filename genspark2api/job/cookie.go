package job

import (
	"genspark2api/common/config"
	logger "genspark2api/common/loggger"
	"math/rand"
	"time"
)

func LoadCookieTask() {
	for {
		source := rand.NewSource(time.Now().UnixNano())
		randomNumber := rand.New(source).Intn(60) // 生成0到60之间的随机整数

		// 计算距离下一个时间间隔
		now := time.Now()
		next := time.Date(now.Year(), now.Month(), now.Day(), 17, 5, 0, 0, now.Location())

		// 如果当前时间已经超过9点，那么等待到第二天的9点
		if now.After(next) {
			next = next.Add(24 * time.Hour)
		}

		delay := next.Sub(now)

		// 等待直到下一个间隔
		time.Sleep(delay + time.Duration(randomNumber)*time.Second)

		logger.SysLog("genspark2api Scheduled LoadCookieTask Task Job Start!")

		config.InitGSCookies()

		logger.SysLog("genspark2api Scheduled LoadCookieTask Task Job  End!")
	}
}
