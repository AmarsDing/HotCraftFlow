package biz

// Radar 链路的 wire 兼容 provider（保留原 wire_gen.go 手工分支语义）。
//
// 背景：RadarTrendingReader 的 demo/真实选择与 GitHubClient 的 tokenFn 注入
// 曾是 wire_gen.go 的手工段（RadarDemoEnabled 运行时分支），导致 wire 无法
// 重新生成。收敛为普通 provider 后 wire 可全图推导，行为不变。
//
// 依赖雷达配置的构造在 biz 只依赖 biz.RadarConfigProvider 接口，绑定在
// service 层完成（RadarConfigService 实现该接口），biz 不反向依赖 service。

// ProvideRadarGitHubClient 用配置的 token 方法构造 GitHub 客户端。
func ProvideRadarGitHubClient(cfg RadarConfigProvider) *GitHubClient {
	return NewGitHubClient(cfg.GitHubToken)
}

// ProvideRadarTrendingReader 合并 demo/真实选择（原手工 `if RadarDemoEnabled()` 分支）。
func ProvideRadarTrendingReader(gh *GitHubClient) RadarTrendingReader {
	if RadarDemoEnabled() {
		return ProvideDemoRadarTrendingReader()
	}
	return NewGitHubRadarTrendingReader(gh)
}
