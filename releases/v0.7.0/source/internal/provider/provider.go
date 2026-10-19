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

	// 必须经过 sqlitedb 而非直接 import modernc.org/sqlite：
	// modernc 的 libc 在首次 getenv 时会复制环境变量，遇到无 "=" 的条目
	// 会直接 os.Exit(1)。sqlitedb 的 init 先把这类条目剔除，是整个程序
	// 唯一允许接触 libc 的地方（见 sqlitedb 包注释与 #2327）。
	_ "github.com/enpiaohj/agentdeck/internal/sqlitedb"
)

// GroupPrefix 是 magpie 路由组的前缀。AgentDeck 无路由组概念，保留常量以免改动调用点。
const GroupPrefix = "group/"

// GrokFastTimes 是 xAI 对其 Grok 快速变体的计价倍数。
const GrokFastTimes = 2

// grokBuildFast 匹配 Grok Build 的快速变体 id。
var grokBuildFast = regexp.MustCompile(`^(grok-[0-9][0-9.]*)-build-fast(?:@[0-9]+k?)?$`)

// grokEffort 匹配带推理档位的 Grok id（grok-4.7-high），捕获其基模型。
var grokEffort = regexp.MustCompile(`^((?:.*/)?grok-[0-9][^/]*?)-(?:minimal|low|medium|high|xhigh|extra-high)$`)

// grokWindow 匹配自带上下文窗口的 Grok id（grok-4.7@500k），捕获其基模型。
var grokWindow = regexp.MustCompile(`^((?:.*/)?grok-[0-9][^/@]*)@[0-9]+k?$`)

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

// pricedNames 是一个模型可能被计价的名字，按优先级排列：自身，然后是
// 去掉推理档位或上下文窗口后缀的基模型。
//
// magpie 的原实现还会为 "codex-auto-review" 追加 "gpt-5.6-luna" 别名，并在
// 候选名中挑选 catalog 真正列出价格的那个（依赖 provider 的 models.dev
// catalog 列表）。AgentDeck 未移植 catalog 列表体系，这里只做后缀剥离，
// 由调用方的 catalog.PricedBy 依次尝试。
func pricedNames(model string) []string {
	out := []string{model}
	lower := strings.ToLower(strings.TrimSpace(model))
	if m := grokWindow.FindStringSubmatch(lower); m != nil {
		out = append(out, m[1])
	}
	if m := grokEffort.FindStringSubmatch(lower); m != nil {
		out = append(out, m[1])
	}
	return out
}

// PricedName 返回该模型在价格目录里最可能的名字。
//
// 对 grok-4.7-high / grok-4.7@500k 这类带档位或窗口后缀的 id，返回可被
// catalog 计价的基模型名；其余情况返回小写化后的原名。
func PricedName(model string) string {
	n := pricedNames(model)
	return n[len(n)-1]
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

// SetModelPrice 记录一个模型的价格，p 为 nil 时删除。
//
// 相对 magpie 的原实现做了简化：只处理 "*/<model>"（任意 provider）与
// "<provider>/<model>" 两种键，不校验 provider 是否真的提供该模型，也不
// 处理 provider 改名后的键追溯。AgentDeck 的定价能力后续单独完善。
func SetModelPrice(id string, p *catalog.Price) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("空的价格键")
	}
	var key string
	if model, every := strings.CutPrefix(id, settings.AnyProvider); every {
		key = AnyPriceKey(model)
	} else {
		key = strings.ToLower(id)
	}

	s := settings.Load()
	if s.ModelPrices == nil {
		s.ModelPrices = map[string]settings.ModelPrice{}
	}
	if p == nil {
		delete(s.ModelPrices, key)
		return settings.Save(s)
	}
	m := settings.StatedPrice(*p)
	if err := settings.CheckModelPrice(key, m); err != nil {
		return err
	}
	s.ModelPrices[key] = m
	return settings.Save(s)
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
