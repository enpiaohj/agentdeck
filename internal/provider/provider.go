// Package provider 是 magpie internal/provider 的最小替代实现。
// 只覆盖 AgentDeck 会话层实际用到的符号：只读打开 + 模型定价。
// 原始实现见 magpie internal/provider（MIT License, Copyright (c) 2026 yetone）。
package provider

import (
	"database/sql"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/enpiaohj/agentdeck/internal/catalog"
	"github.com/enpiaohj/agentdeck/internal/settings"

	_ "modernc.org/sqlite"
)

// GroupPrefix 是 magpie 路由组的前缀。AgentDeck 无路由组概念，保留常量以免改动调用点。
const GroupPrefix = "group/"

// GrokFastTimes 是 xAI 对其 Grok 快速变体的计价倍数。
const GrokFastTimes = 2

// grokBuildFast 匹配 Grok Build 的快速变体 id。
var grokBuildFast = regexp.MustCompile(`^(grok-[0-9][0-9.]*)-build-fast(?:@[0-9]+k?)?$`)

// OpenReadOnly 以只读方式打开另一个应用的 SQLite 数据库，不写入。
func OpenReadOnly(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Path: sqlitePath(path), RawQuery: "mode=ro&_pragma=busy_timeout(3000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, errors.New("can't open " + path + ": " + err.Error())
	}
	return db, nil
}

// sqlitePath 把 Windows 路径转成 file: URL 可用的形式。
func sqlitePath(path string) string {
	p := strings.ReplaceAll(path, `\`, "/")
	if len(p) >= 2 && p[1] == ':' {
		p = "/" + p
	}
	return p
}

// AnyPriceKey 是 ModelPrices 中「任意 provider」的价格键。
func AnyPriceKey(model string) string {
	return settings.AnyProvider + strings.ToLower(strings.TrimSpace(model))
}

// PricedName 返回该模型在价格目录里最可能的名字。
func PricedName(model string) string {
	return strings.ToLower(strings.TrimSpace(model))
}

// GrokBuildFast 判断 id 是否为 Grok Build 的快速变体，并返回其基模型 id。
func GrokBuildFast(id string) (string, bool) {
	m := grokBuildFast.FindStringSubmatch(strings.ToLower(strings.TrimSpace(id)))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// EffectivePriceIn 取该模型的有效输入价。
func EffectivePriceIn(s settings.Settings, providerID, model string) (catalog.Price, bool) {
	bare := strings.ToLower(strings.TrimSpace(model))
	if p, ok := s.ModelPrices[AnyPriceKey(model)]; ok {
		return priceOf(p), true
	}
	if p, ok := s.ModelPrices[providerID+"/"+bare]; ok {
		return priceOf(p), true
	}
	return catalog.Price{}, false
}

// priceOf 把 ModelPrice 的指针字段解引用成 catalog.Price。
func priceOf(p settings.ModelPrice) catalog.Price {
	var out catalog.Price
	if p.Input != nil {
		out.Input = *p.Input
	}
	if p.Output != nil {
		out.Output = *p.Output
	}
	if p.CacheRead != nil {
		out.CacheRead = *p.CacheRead
	}
	if p.CacheWrite != nil {
		out.CacheWrite = *p.CacheWrite
	}
	if p.CacheWrite1h != nil {
		out.CacheWrite1h = *p.CacheWrite1h
	}
	out.Tiers = p.Tiers
	return out
}
