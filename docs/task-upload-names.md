# task-upload-names: 附件上传名兼容（网站扩展名策略）

> 状态: 已实现，待 review（review 结论已全部采纳：大小写精确匹配、probe 电池守卫与边界匹配、改名全量表面化、去重收拢到 lp、`foo.` 归一）
> 依据: 2026-09-13 两次 live probe 实测（本文件 §2）+ 用户当日 review 事故报告
> 作者: architect（玻尔）

## 1. 背景与事故

09-13 的 shell-block 交付 review 过程中：提交 `a84ba1a` 修改了仓库根 `.gitignore`，
`code_review` 按既有设计把每个变更文件全文作为附件上传（`encodeAttachmentName` 编码
仓库路径），**`.gitignore` 上传后卡片变为「该格式暂不支持」（站点服务端错误），发送被
阻断**（站点提示「请删除异常文件再发送」），用户只能手工在浏览器里删除该附件，review
才得以发出。

用户要求：查明哪些扩展名可以 attach；对不可 attach 的扩展名（如 `.gitignore`）做处理
（不 attach 或按 txt attach），否则每次命中都会中断 review。

## 2. 事实（live probe 实测，2026-09-13）

站点（chat.deepseek.com）对上传文件名做**扩展名**检查，且客户端过滤列表与服务端接受
列表不一致，存在两种失败形态：

- **服务端拒绝（卡片 +「该格式暂不支持」→ 阻断发送，需手工删除）**：
  - `.gitignore`（扩展名 `gitignore`）——事故复现成功。
- **客户端静默丢弃（无卡片，仅 toast）**：
  - 无扩展名：`Makefile`、`Dockerfile`、`LICENSE`；
  - 扩展名不在客户端列表：`.gitattributes`(`gitattributes`)、`go.sum`(`sum`)、
    `go.work`(`work`)、`.env`(`env`)。（注意 `go.mod`/`mod` 可传，`sum`/`work` 不可。）
- **正常接受（卡片显示大小）**：`md go patch txt yml yaml json toml ini conf cfg log
  csv tsv lock mod sh py rs ts js css scss html xml sql proto org el mk rb pl c cpp h
  java cs kt swift php`，以及既有已知可传的图片（png/jpg/jpeg/gif/webp/bmp）与 pdf。
- **改名验证（关键）**：`.gitignore.txt`、`gitignore.txt`、`Makefile.txt` 全部正常接收
  （TXT 卡片）——**前导点无碍，问题只在扩展名；「加 `.txt` 后缀」是通用解**。
- 反例：`icon.svg` 被当图片上传，文字提取为空（「未提取到文字」），对评审无用。

复现/取证脚本：`internal/lp/upload_probe_live_test.go`（本轮侦察产物，见 §3.4，
改造成正式 gated live test 保留）。

## 3. 设计

原则：**扩展名策略收敛到 lp 单点**（唯一的站点知识源），code_review 在装配期复用同一
函数并保持专家可读性（改名要显式可见），所有上传调用方（webchat CLI `--attach`、
ask_expert 图片、AskExpertWithRoleFiles→code_review）自动获得保护。

### 3.1 lp 层

1. 新增 `internal/lp/uploadname.go`：
   - `verifiedUploadExts`（map/set）：§2「正常接受」列表 + 图片/pdf；注释写明来源日期与
     复核方法（跑 live probe）。
   - `SafeUploadName(name string) (string, bool)`：纯函数。取 `filepath.Ext`，**精确、大小写
     敏感**匹配集合（集合只含探测过的小写形式）：命中 → `(name, false)`；否则 →
     `(name+".txt", true)`。无扩展名同样命中改名分支（`Makefile` → `Makefile.txt`）；
     未探测过的拼写按 fail-safe 改名（`README.MD` → `README.MD.txt`）。**尾点（一个或多个）
     全部去掉**后重新推导扩展：`foo.` → `foo.txt`、`foo.txt.` → `foo.txt`、`a..` → `a.txt`
     （不叠出 `foo..txt`/`foo.txt.txt`/`a..txt`，且计入 renamed）。整名恰为已验证扩展名的隐藏文件
     （`.txt`/`.md`）按扩展名判定放行（已知且刻意；该结论由扩展名模型推出，
     并非直接实测——probe 电池已换入裸 `.md` 候选，下一次探测轮即可确认）。
2. `internal/lp/webchat.go` 的 `webchatUpload`：在 `validateWebAttachments`（50 个/100MB
   限额，仍按原文件校验）之后、上传之前统一归一化（内部函数，建议名
   `prepareUploadAttachments`）：
   - 输入原路径列表，输出「上传路径列表 + 改名备注 + cleanup」；
   - 需要改名的文件在临时目录生成副本（包级缝 `uploadTempDir`，默认 `os.MkdirTemp("", "dscli-upload-")`；
     文件 0600），上传用副本路径，`defer cleanup()`（无改名则 cleanup 为 nil，零副作用）；
     出错路径先清理临时目录再返回；
   - 去重：以最终基名为键（含未改名项），冲突时用 `lp.UniqueUploadName` 在扩展名前插 `_N`
     （`x.txt` → `x_2.txt`；`Makefile` → 改名 `Makefile.txt` → 冲突 `Makefile_2.txt`），
     保证最终名仍以 `.txt` 结尾、不会被 lp 二次改名；该函数导出，code_review 侧复用同一实现；
   - 每个改名打印一行 stderr 备注（中文，风格对齐现有 `📎` 行），例如：
     `📎 .gitignore 的扩展名网站不支持，已按 .gitignore.txt 上传（内容不变）`；
   - 探测输入框（direct path）与点击上传（chooser fallback）两条路径都必须使用归一化后的
     路径（`webchatSetUploadFiles` 的就绪检查按传入路径的 base 名计数，自然一致）。
   - login 重试会重新走 `webchatUpload`，各自建副本/清理，可接受。
3. `webchat_cmd.go` Long 帮助「上传限制」段补一句改名策略说明。

### 3.2 code_review 层

1. `assembleReviewAttachments`（`internal/toolcall/ask/code_review.go`）：kept 文件装配时
   ```go
   encoded := encodeAttachmentName(c.path)
   safe, _ := lp.SafeUploadName(encoded)
   name := lp.UniqueUploadName(usedNames, safe)   // 去重后才是最终名
   // 复制成功、且最终名与编码名不同时才记录（.txt 改名与去重后缀都算）
   if name != encoded { plan.Renamed = append(plan.Renamed, c.path+" → "+name) }
   ```
   然后 `copyReviewFile(dir, name, ...)`；复制失败则不记录改名（仅计入 NotAttached）。
   名字在复制前已由 `UniqueUploadName` 占用，失败后不回收（保留位语义，调用处有注释）。
   固定附件（review-guide.md / changes.patch / AGENTS.md / gocyclo.txt）均为已验证扩展名，
   保持原样。
2. 去重收拢到 `lp.UniqueUploadName`（导出）：数字后缀**插在扩展名前**（`x.go` → `x_2.go`；
   `Makefile.txt` → `Makefile_2.txt`），ask 侧不再有本地实现。
3. `reviewPlan` 新增字段 `Renamed []string`（`repo path → attachment name`，排序）。
4. `buildReviewMessage`：
   - 编码说明句补 `.txt` 约定（英文，专家可读）：说明「原名扩展名站点不接受时会追加
     `.txt`（内容不变）」，例如 `".gitignore.txt"` 对应 `".gitignore"`；
   - Coverage 区新增一行（仅当 `len(plan.Renamed) > 0`）：
     `- Upload-name adjustments (repo path → attachment name, content unchanged): .gitignore → .gitignore.txt, …`
     （复用 `cappedList` 截断；左侧 repo 路径、右侧附件名）。
   - 本地无需新增打印：消息整体已随 `📤` 输出，专家覆盖区即用户可见。
5. `internal/toolcall/ask/code_review.md`（工具说明）Context 段补 `.txt` 约定一句。

### 3.3 文档

- `AGENTS.md`：`internal/lp/` 行补充上传名归一化一句话（`SafeUploadName`、未知扩展名
  加 `.txt`、2026-09-13 live probe 建立的已验证集合）。

### 3.4 live probe 测试（保留 + 打磨）

`internal/lp/upload_probe_live_test.go`：去掉「temporary」措辞，改为正式 gated live test：
- `DSCLI_LIVE_UPLOAD_PROBE=1` 才运行（无 env 必须 skip；CI/日常 `go test ./...` 不受影响）；
- 注释说明用途：复核 `verifiedUploadExts`（重跑电池 → 按输出更新集合）；
- 保留候选清单（对照组 + `.gitignore` 失败组 + 改名验证组）与日志式输出（不做脆弱断言）。
- **开发期不要真跑**（需 Chrome 登录、会打开浏览器窗口、往站点传文件）；只要保证编译通过、
  无 env 时走 skip 分支即可。

### 3.5 测试

- lp 单测（新文件或 `webchat_test.go`）：
  - `SafeUploadName` 表驱动：`.gitignore`→`.gitignore.txt`；`Makefile`→`Makefile.txt`；
    `go.sum`→`go.sum.txt`；`Makefile.txt`/`main.go`/`a.txt`/`.github__x.yml` 保持不变；
    **未探测的大写形式 fail-safe 改名**（`README.MD` → `README.MD.txt`、`Icon.PNG` →
    `Icon.PNG.txt`）；`foo.`→`foo.txt`、`foo.txt.`→`foo.txt`、`a..`/`a...`→`a.txt`；
    `icon.svg`→`icon.svg.txt`。
  - `prepareUploadAttachments`：改名者生成 0600 副本且内容一致；未改名者返回原路径；
    无改名时 cleanup 为 nil；清理后副本消失；重名去重（`x.txt` + `x` → `x.txt` + `x_2.txt`，
    断言重名原因的中文备注）；出错路径（源缺失 / 复制失败）清理临时目录（用 `uploadTempDir`
    测试缝把计数限定在测试目录内）。
- code_review 单测：
  - 装配含 `.gitignore`、`Makefile`、`go.sum`、`main.go` 的变更集 → 断言附件文件名与
    `plan.Renamed`；
  - 编码名冲突（`a/b__c.go` 与 `a__b/c.go`）→ 第二个得 `_2` 且计入 `plan.Renamed`，
    并断言 `buildReviewMessage` 输出含该条目；
  - 复制失败 → 不记录改名、列入 NotAttached；
  - `lp.UniqueUploadName` 新行为（ask 侧本地实现已删除）；
  - `buildReviewMessage`：新增改名行/新句子断言；既有断言不得破坏。
- 全量：`go test ./...` + `make fmt-check` 通过（提交前）。

### 3.6 非目标

- 不改 DSML 通道、shell-block 通道；不改上传限额（50 个/100MB）；
- 不改 ask_expert 文本内联行为（其文本附件不上传，图片本就可传）；
- 不新增配置开关；不做站点端自动探测（live probe 手动）。

## 4. 验收

- [x] `go test ./...` 全绿；`make fmt-check` 通过；
- [x] live probe 无 env 时 skip、`go vet ./...` 干净；
- [x] 一个英文 commit（`fix(lp): upload site-incompatible attachment names with a .txt suffix`），工作区干净。
- [x] 复审修复 commit（大小写精确匹配、probe 电池 ≤50 守卫、`plan.Renamed` 全量记录、去重收拢 `lp.UniqueUploadName`）。

## 5. 备注

- 「不 attach vs 按 txt attach」：本设计选**按 txt attach**（内容保留，专家可见；skip 会
  丢全文）。改名对评审是显式可见的（Coverage 行 + 编码说明句）。
- `verifiedUploadExts` 是「已验证可原样上传」的保守集合；站点列表会漂移，未知一律 `.txt`
  （fail-safe）。集合未来只增不减地按 probe 结果维护。
- 事故现场：`a84ba1a`（改 `.gitignore`）+ `since=-3` 的 review；复现候选已固化在 probe。
- 扩展名匹配**精确、大小写敏感**（fail-safe）：`README.MD`、`Icon.PNG` 之类未探测过的拼写一律改名
  （`README.MD` → `README.MD.txt`）。若未来 probe 证实站点大小写不敏感，再据证据放宽
  `SafeUploadName` 并同步注释。probe 电池已含大写案例，且整体 ≤ `WebUploadMaxFiles`（有守卫测试）。

## 6. 操作须知（给 dev）

- 仓库根即当前工作目录（`codedev` 工作树）。工作区现有一个未跟踪文件
  `internal/lp/upload_probe_live_test.go`（本轮侦察产物），按 §3.4 打磨后保留并提交；
  不要把它删掉。
- **不要运行 live probe**（需要 Chrome + 站点登录，会打开浏览器窗口并上传文件到站点）；
  只需保证：无 `DSCLI_LIVE_UPLOAD_PROBE` 环境变量时它被 skip、包编译/`go vet` 通过。
- 不要提交本地杂物（`output*.txt`、`script*.sh`、`*.patch` 等，多数已在 .gitignore）。
- 开发期间可用 `make dev-test`；提交前必须 `go test ./...` + `make fmt-check` 全绿。
- 本文档（`docs/task-upload-names.md`）随实现一并提交，状态行改为「已实现，待 review」
  （参考 `docs/task-shell-block.md` 的先例）。
- commit 信息用英文；工作区必须干净后返回。
