# SWAVES

![swaves.png](https://i.see.you/2026/04/24/q8Dx/swaves.png)

> SWAVES 是一个简单、安静、轻量级的博客/内容管理系统

## 技术特性

* 基于 golang + fiber 构建高性能的高性能服务端
* 所有数据存储在一个单 sqlite3 数据库文件
* 安装文件永远只会是一个不超过 50M 的可执行文件
* 后台 master + worker 进程守护机制保证持续在线不宕机

## 为什么使用它

* 内容存储 markdown 源文件，支持可视化/代码两种编辑模式
* 数据永远在自己的掌握中，不捆绑其它平台
* 完整的数据导入、导出、备份、恢复功能

## 可能不适用

* 庞大的内容管理系统
* 在线协作文档系统
* 多用户系统
* 高并发、大流量的分布式部署系统(性能压测)

## 核心功能

- 内容发布：支持文章、页面、分类、标签等常见内容组织方式
- 站点管理：支持站点信息、路径规则、后台入口、阅读展示等基础配置
- 主题系统：支持主题编辑、复制、导入导出与切换
- 链接治理：支持重定向规则管理，方便旧链接迁移和站点结构调整
- 数据安全：支持导入导出、备份恢复，降低迁移和误操作成本
- 互动与运营：支持评论、通知、点赞、任务等日常站点运营能力
- 特殊内容：支持加密文章，适合需要受限访问的内容场景

## 文档导航

- [CLI 参数参考](./cli-reference.md)
- [环境变量参考](./env-reference.md)
- [模板约定](./template-conventions.md)
- [模板 API](./template_api.md)
- [SUI dash 路由映射](./sui-dash-route-mapping.md)
- [性能测试说明](./perf-test.md)

## 立即体验

- 下载预编译 binary：前往 [Releases](https://github.com/keelii/swaves/releases)
- 启动服务：

```bash
# Linux / macOS
curl -fsSL https://swaves.io/install.sh | sh
```
