#!/bin/bash

set -e

# cmd
mkdir -p cmd/swv

# internal core
mkdir -p internal/db
mkdir -p internal/config
mkdir -p internal/render
mkdir -p internal/web
mkdir -p internal/task
mkdir -p internal/backup

# frontend templates & static
mkdir -p web/templates
mkdir -p web/static

echo "✅ swaves 目录结构创建完成"
