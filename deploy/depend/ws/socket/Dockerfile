# 基础镜像busybox可能缺少必要的运行时依赖库，导致服务启动后立即崩溃，导致端口无法保持监听
# 改用更完整的alpine镜像并添加基础库依赖 同时服务绑定地址显式指定为0.0.0.0
# FROM alpine:3.18  
# RUN apk add --no-cache libc6-compat
FROM docker.1panel.live/library/busybox
WORKDIR /app
COPY ./ws/socket/socket /app/socket
RUN chmod +x /app/socket