package db

import (
	"encoding/json"
	"time"
)

const DefaultThemeTemplateCode = "default-theme-template"

func defaultThemeTemplateFiles() map[string]string {
	return map[string]string{
		"layout_main.html": `<!DOCTYPE html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <title>{{ Title or Settings("site_title") }}</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
</head>
<body>
  <header>
    <h1>{{ Settings("site_title") }}</h1>
    <p>{{ Settings("site_description") }}</p>
  </header>
  <main>
    {% block content %}{% endblock %}
  </main>
</body>
</html>
`,
		"home.html": `{% extends "layout_main.html" %}
{% block content %}
<h2>首页</h2>
<ul>
  {% for article in Articles %}
  <li><a href="{{ article.PermLink }}">{{ article.Post.Title }}</a></li>
  {% else %}
  <li>暂无文章</li>
  {% endfor %}
</ul>
{% endblock %}
`,
		"post.html": `{% extends "layout_main.html" %}
{% block content %}
<article>
  <h2>{{ Post.Post.Title }}</h2>
  <div>{{ Post.HTML | safe }}</div>
</article>
{% endblock %}
`,
		"list.html": `{% extends "layout_main.html" %}
{% block content %}
<h2>{{ Title }}</h2>
<ul>
  {% for item in List %}
  <li><a href="{{ item.PermLink }}">{{ item.Name }}</a></li>
  {% else %}
  <li>暂无内容</li>
  {% endfor %}
</ul>
{% endblock %}
`,
		"detail.html": `{% extends "layout_main.html" %}
{% block content %}
<h2>{{ Entity.Name }}</h2>
<ul>
  {% for item in List %}
  <li><a href="{{ item.PermLink }}">{{ item.Post.Title }}</a></li>
  {% else %}
  <li>暂无内容</li>
  {% endfor %}
</ul>
{% endblock %}
`,
		"404.html": `{% extends "layout_main.html" %}
{% block content %}
<h2>404</h2>
<p>页面不存在。</p>
{% endblock %}
`,
		"error.html": `{% extends "layout_main.html" %}
{% block content %}
<h2>错误</h2>
<p>{{ Msg or "页面暂时不可用。" }}</p>
{% endblock %}
`,
	}
}

func newDefaultThemeTemplate() (*Theme, error) {
	files, err := json.Marshal(defaultThemeTemplateFiles())
	if err != nil {
		return nil, WrapInternalErr("newDefaultThemeTemplate.Marshal", err)
	}

	nowUnix := time.Now().Unix()
	return &Theme{
		Name:        "新建主题模板",
		Code:        DefaultThemeTemplateCode,
		Description: "用于创建新主题的最小模板",
		Author:      "swaves",
		Files:       string(files),
		CurrentFile: "home.html",
		Status:      "draft",
		IsBuiltin:   1,
		Version:     1,
		CreatedAt:   nowUnix,
		UpdatedAt:   nowUnix,
	}, nil
}

func EnsureDefaultThemeTemplate(db *DB) error {
	_, err := GetThemeByCode(db, DefaultThemeTemplateCode)
	if err == nil {
		return nil
	}
	if !IsErrNotFound(err) {
		return err
	}

	theme, err := newDefaultThemeTemplate()
	if err != nil {
		return err
	}
	_, err = CreateTheme(db, theme)
	return err
}
