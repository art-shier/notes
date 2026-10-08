#!/usr/bin/env bash
# Legacy ConfigHub generation is retired; preserve every existing config file.
printf '[拾记] ConfigHub配置生成已移除。请在ctl配置DATABASE_URL或DB_HOST/DB_USER/DB_PASSWORD，再执行ctl install/upgrade notes --prod。已有私有配置文件保持不变。\n' >&2
exit 1
