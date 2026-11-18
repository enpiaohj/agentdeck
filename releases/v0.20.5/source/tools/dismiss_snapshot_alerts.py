# -*- coding: utf-8 -*-
"""驳回落在发布快照里的 Dependabot 告警。

releases/vX.Y.Z/source/ 是不可变的发布快照，每个都带一份 go.mod 与
package-lock.json。GitHub 把它们当作独立项目扫描，同一个依赖问题会按快照
数量重复报告；快照不参与构建，也不允许修改，所以这些告警只能驳回。
当前代码（仓库根的 go.mod、frontend/package-lock.json）的告警不在此列，
必须通过升级依赖解决。

用法（需要已登录的 gh CLI）：
    python -I tools/dismiss_snapshot_alerts.py            # 只列出，不改动
    python -I tools/dismiss_snapshot_alerts.py --apply    # 实际驳回
"""
import json
import re
import subprocess
import sys

sys.stdout.reconfigure(encoding='utf-8')

REPO = 'enpiaohj/agentdeck'
SNAPSHOT = re.compile(r'^releases/v\d+\.\d+\.\d+/source/')
COMMENT = ('该清单属于 releases/ 下不可变的发布快照，不参与构建、不接受修改；'
           '当前代码中的同一依赖按正常流程升级。')


def gh(*args):
    return subprocess.run(['gh', *args], capture_output=True, text=True, encoding='utf-8', check=True).stdout


def open_alerts():
    # --paginate 会把每页的 JSON 数组首尾相接输出，用 --jq 逐条展开成一行一个对象
    out = gh('api', f'repos/{REPO}/dependabot/alerts?state=open&per_page=100', '--paginate',
             '--jq', '.[] | {number, path: .dependency.manifest_path, pkg: .dependency.package.name, '
                     'severity: .security_advisory.severity}')
    return [json.loads(line) for line in out.splitlines() if line.strip()]


def main():
    apply = '--apply' in sys.argv[1:]
    alerts = open_alerts()
    targets = [a for a in alerts if SNAPSHOT.match(a['path'] or '')]
    live = [a for a in alerts if a not in targets]
    print(f'未关闭告警 {len(alerts)} 个：发布快照 {len(targets)} 个，当前代码 {len(live)} 个')
    for a in live:
        print(f"  当前代码（需升级依赖）#{a['number']} {a['severity']} {a['pkg']} {a['path']}")
    if not apply:
        print('预览模式：未改动。加 --apply 驳回上述发布快照告警。')
        return
    failed = 0
    for a in targets:
        try:
            gh('api', '-X', 'PATCH', f"repos/{REPO}/dependabot/alerts/{a['number']}",
               '-f', 'state=dismissed', '-f', 'dismissed_reason=not_used', '-f', f'dismissed_comment={COMMENT}')
        except subprocess.CalledProcessError as error:
            failed += 1
            print(f"  驳回失败 #{a['number']}：{error.stderr.strip()}")
    print(f'已驳回 {len(targets) - failed} 个，失败 {failed} 个')
    sys.exit(1 if failed else 0)


main()
