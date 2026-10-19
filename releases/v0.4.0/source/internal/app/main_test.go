package app

import (
	"testing"

	"github.com/enpiaohj/agentdeck/internal/testenv"
)

// 隔离所有用户目录，测试不得读取或改写真实会话、索引和配置。
func TestMain(m *testing.M) { testenv.Main(m) }
