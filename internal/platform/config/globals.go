package config

import "time"

const LoginDashName = "dash"

const LoginCookieName = "swv_login"

const LoginSessionExpire = time.Hour * 24 * 30 * 365

const LoginRoutePath = "/dash/login"

const GlobalSettingKey = "settings"

const BaseTimeFormat = "2006-01-02 15:04:05"

// const ArticleTimeFormat = "2006-01-02 15:04:05"
const ArticleTimeFormat = "2006年1月2日"

const GravatarDomain = "https://cravatar.cn"
