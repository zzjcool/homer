# homer QA 网页环境
#
# 目标：`make qa-up` 从源码一键启动 hub 控制台 + idle agent 容器 + 浏览器，
# AI QA 直接操作网页界面完成测试。

QA_COMPOSE := docker compose -f qa/env/docker-compose.yml
QA_HUB := http://127.0.0.1:17760
QA_PASSWORD := qa-admin-password

.PHONY: qa-up qa-down qa-logs qa-restart qa-status qa-help

## qa-up: 源码构建并启动整个 QA 环境（hub + 3 个 idle box + 浏览器）
qa-up:
	$(QA_COMPOSE) up -d --build
	@echo "---- 等待 hub 就绪 ----"
	@for i in $$(seq 1 60); do \
		curl -sf $(QA_HUB)/api/health >/dev/null 2>&1 && break; \
		sleep 1; \
	done
	@echo "hub 就绪: $(QA_HUB)"
	@echo "浏览器 CDP: ws://127.0.0.1:3000"
	@echo "管理员密码: $(QA_PASSWORD)"
	@echo "API 断言: curl -H 'Authorization: Bearer qa-hub-token' $(QA_HUB)/api/agents"

## qa-down: 停止并删除容器与网络（数据在 volume/镜像外，重建即全新）
qa-down:
	$(QA_COMPOSE) down --remove-orphans

## qa-restart: 重建环境（down + up，hub 状态清零）
qa-restart: qa-down qa-up

## qa-logs: 跟随 hub 日志
qa-logs:
	$(QA_COMPOSE) logs -f hub

## qa-status: 查看容器状态
qa-status:
	$(QA_COMPOSE) ps
