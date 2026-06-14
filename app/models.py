from __future__ import annotations

from dataclasses import asdict, dataclass, field
from datetime import datetime, timezone
from enum import Enum
from typing import Any


class StepId(str, Enum):
    PRECHECK = "precheck"
    BACKUP_TRIGGER = "backup_trigger"
    BACKUP_PACK = "backup_pack"
    BACKUP_UPLOAD = "backup_upload"
    BACKUP_DONE = "backup_done"
    PROD_PRE_BACKUP = "prod_pre_backup"
    PROD_SQL = "prod_sql"
    PROD_NACOS = "prod_nacos"
    PROD_CONFIG = "prod_config"
    PROD_CODE = "prod_code"
    PROD_RESTART = "prod_restart"
    PROD_VERIFY = "prod_verify"
    FINISHED = "finished"


class StepStatus(str, Enum):
    PENDING = "pending"
    RUNNING = "running"
    SUCCESS = "success"
    FAILED = "failed"
    SKIPPED = "skipped"


STEP_TITLES: dict[StepId, str] = {
    StepId.PRECHECK: "预检查（SSH / 脚本 / 锁）",
    StepId.BACKUP_TRIGGER: "25 触发立即备份",
    StepId.BACKUP_PACK: "25 等待打包完成",
    StepId.BACKUP_UPLOAD: "25 等待上传网盘",
    StepId.BACKUP_DONE: "25 备份上传完成",
    StepId.PROD_PRE_BACKUP: "149 变更前备份（扩展）",
    StepId.PROD_SQL: "149 增量 SQL（扩展）",
    StepId.PROD_NACOS: "149 Nacos 同步（扩展）",
    StepId.PROD_CONFIG: "149 配置同步（扩展）",
    StepId.PROD_CODE: "149 下载网盘 + 部署到绿环境",
    StepId.PROD_RESTART: "149 重启绿环境后端",
    StepId.PROD_VERIFY: "149 绿环境 HTTP 验收",
    StepId.FINISHED: "一键部署绿环境完成",
}

# 当前 v1 走 osh-green-code-sync.sh → 仅 /opt/osh-green；蓝项目不可触碰
CODE_SYNC_SKIP: set[StepId] = {
    StepId.PROD_PRE_BACKUP,
    StepId.PROD_SQL,
    StepId.PROD_NACOS,
    StepId.PROD_CONFIG,
}

STEP_ORDER: list[StepId] = list(STEP_TITLES.keys())


@dataclass
class Step:
    id: StepId
    title: str
    status: StepStatus = StepStatus.PENDING
    message: str = ""
    started_at: str | None = None
    finished_at: str | None = None


@dataclass
class Run:
    id: str
    mode: str
    status: StepStatus
    created_at: str
    steps: list[Step] = field(default_factory=list)
    log: list[str] = field(default_factory=list)

    def append_log(self, line: str) -> None:
        ts = datetime.now(timezone.utc).strftime("%H:%M:%S")
        self.log.append(f"[{ts}] {line}")
        if len(self.log) > 800:
            self.log = self.log[-800:]

    def to_dict(self) -> dict[str, Any]:
        return {
            **{k: v for k, v in asdict(self).items() if k != "steps"},
            "steps": [asdict(s) for s in self.steps],
        }


def build_steps(mode: str, *, use_release: bool = False) -> list[Step]:
    skip: set[StepId] = set()
    if mode == "skip_backup":
        skip |= {
            StepId.BACKUP_TRIGGER,
            StepId.BACKUP_PACK,
            StepId.BACKUP_UPLOAD,
            StepId.BACKUP_DONE,
        }
    elif mode == "code_only":
        skip |= {
            StepId.BACKUP_TRIGGER,
            StepId.BACKUP_PACK,
            StepId.BACKUP_UPLOAD,
            StepId.BACKUP_DONE,
        }

    # v1 默认 code-sync；仅当 149 已安装 osh-prod-release.sh 时才展示扩展步骤
    if not use_release:
        skip |= CODE_SYNC_SKIP

    steps: list[Step] = []
    for sid in STEP_ORDER:
        status = StepStatus.SKIPPED if sid in skip else StepStatus.PENDING
        steps.append(Step(id=sid, title=STEP_TITLES[sid], status=status))
    return steps
