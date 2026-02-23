package config

import "strings"

// TechBlogCategoryTranslations 用于中文技术博客分类的固定翻译（值建议直接用于 slug）。
// 命中该表时可优先使用；未命中时再调用翻译 API。
var TechBlogCategoryTranslations = map[string]string{
	// 通用主题
	"技术":    "technology",
	"科技":    "tech",
	"科技资讯":  "tech-news",
	"科技新闻":  "tech-news",
	"技术资讯":  "tech-news",
	"前沿科技":  "emerging-tech",
	"互联网":   "internet",
	"互联网产品": "internet-products",
	"行业观察":  "industry-insights",
	"趋势":    "trends",

	// 内容类型
	"教程":   "tutorials",
	"实战":   "hands-on",
	"指南":   "guides",
	"最佳实践": "best-practices",
	"经验分享": "experience-sharing",
	"踩坑记录": "lessons-learned",
	"学习笔记": "study-notes",
	"读书笔记": "reading-notes",
	"源码":   "source-code",
	"源码分析": "source-code-analysis",

	// 开发与架构
	"编程":    "programming",
	"程序设计":  "programming",
	"开发":    "development",
	"软件开发":  "software-development",
	"全栈":    "full-stack",
	"全栈开发":  "full-stack-development",
	"后端":    "backend",
	"后端开发":  "backend-development",
	"前端":    "frontend",
	"前端开发":  "frontend-development",
	"前端工程化": "frontend-tooling",
	"web开发": "web-development",
	"架构":    "architecture",
	"架构设计":  "architecture-design",
	"系统设计":  "system-design",
	"分布式":   "distributed-systems",
	"微服务":   "microservices",
	"中间件":   "middleware",
	"高并发":   "high-concurrency",
	"高可用":   "high-availability",
	"容错":    "fault-tolerance",
	"可观测性":  "observability",
	"性能优化":  "performance-optimization",
	"性能调优":  "performance-tuning",
	"调试":    "debugging",
	"故障排查":  "troubleshooting",

	// 语言与框架
	"go":         "go",
	"golang":     "go",
	"rust":       "rust",
	"java":       "java",
	"python":     "python",
	"javascript": "javascript",
	"typescript": "typescript",
	"node.js":    "nodejs",
	"nodejs":     "nodejs",
	"react":      "react",
	"vue":        "vue",
	"angular":    "angular",
	"php":        "php",
	"ruby":       "ruby",
	"kotlin":     "kotlin",
	"swift":      "swift",
	"scala":      "scala",
	"c":          "c",
	"c++":        "cpp",
	"cpp":        "cpp",
	"c#":         "csharp",
	"csharp":     "csharp",
	"api设计":      "api-design",
	"接口设计":       "api-design",
	"设计模式":       "design-patterns",
	"算法":         "algorithms",
	"数据结构":       "data-structures",

	// 移动端
	"移动开发":         "mobile-development",
	"android":      "android",
	"android开发":    "android-development",
	"ios":          "ios",
	"ios开发":        "ios-development",
	"flutter":      "flutter",
	"react native": "react-native",
	"跨平台":          "cross-platform",

	// 基础设施与运维
	"devops":     "devops",
	"运维":         "operations",
	"sre":        "sre",
	"云计算":        "cloud-computing",
	"云原生":        "cloud-native",
	"容器":         "containers",
	"docker":     "docker",
	"kubernetes": "kubernetes",
	"k8s":        "kubernetes",
	"服务网格":       "service-mesh",
	"持续集成":       "continuous-integration",
	"持续交付":       "continuous-delivery",
	"ci/cd":      "cicd",
	"cicd":       "cicd",
	"自动化":        "automation",
	"测试":         "testing",
	"自动化测试":      "test-automation",
	"单元测试":       "unit-testing",
	"集成测试":       "integration-testing",
	"工程效率":       "engineering-productivity",

	// 系统与网络
	"linux": "linux",
	"操作系统":  "operating-systems",
	"网络":    "networking",
	"计算机网络": "computer-networking",
	"服务器":   "servers",
	"数据库":   "database",
	"sql":   "sql",
	"nosql": "nosql",
	"缓存":    "caching",
	"消息队列":  "message-queue",
	"搜索":    "search",
	"全文检索":  "full-text-search",

	// 安全
	"安全":   "security",
	"网络安全": "cybersecurity",
	"信息安全": "information-security",
	"应用安全": "application-security",
	"数据安全": "data-security",
	"隐私保护": "privacy",

	// 数据与 AI
	"数据":     "data",
	"数据分析":   "data-analysis",
	"数据工程":   "data-engineering",
	"数据科学":   "data-science",
	"大数据":    "big-data",
	"人工智能":   "artificial-intelligence",
	"ai":     "artificial-intelligence",
	"机器学习":   "machine-learning",
	"深度学习":   "deep-learning",
	"大模型":    "large-language-models",
	"llm":    "large-language-models",
	"自然语言处理": "nlp",
	"nlp":    "nlp",
	"计算机视觉":  "computer-vision",
	"cv":     "computer-vision",
	"推荐系统":   "recommendation-systems",

	// 开源、工具与职业
	"开源":    "open-source",
	"开源项目":  "open-source-projects",
	"工具":    "tools",
	"开发工具":  "developer-tools",
	"效率工具":  "productivity-tools",
	"git":   "git",
	"版本控制":  "version-control",
	"区块链":   "blockchain",
	"物联网":   "internet-of-things",
	"iot":   "internet-of-things",
	"边缘计算":  "edge-computing",
	"产品":    "product",
	"产品设计":  "product-design",
	"创业":    "startup",
	"职场":    "career",
	"程序员成长": "developer-growth",
	"面试":    "interviews",
	"团队管理":  "engineering-management",
}

func LookupTechBlogCategoryTranslation(name string) (string, bool) {
	key := normalizeTechBlogCategoryKey(name)
	if key == "" {
		return "", false
	}
	v, ok := TechBlogCategoryTranslations[key]
	return v, ok
}

func normalizeTechBlogCategoryKey(name string) string {
	name = strings.ReplaceAll(name, "　", " ")
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	return strings.Join(strings.Fields(name), " ")
}
