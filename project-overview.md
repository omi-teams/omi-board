# omi-board — Ý tưởng và mục tiêu dự án

Ngày tổng hợp: 2026-11-07, múi giờ Asia/Bangkok. Tài liệu đầu vào cho `omi-idea`: các mục "Câu hỏi mở" là những gì `omi-idea` cần hỏi PO; mỗi câu đã có đề xuất mặc định.

## 1. Ý tưởng cốt lõi

**omi-board là ứng dụng web quản lý dự án cho omi: mỗi dự án liên kết một repo GitHub, công việc là các ticket xếp thành cây, PO và thành viên làm việc trên đó, còn squad AI của omi đọc và ghi ticket qua một giao diện chính thức.**

omi-board thay thế `omi board` hiện tại, vốn là trang web chỉ đọc, dữ liệu lấy từ file Markdown trong repo omi-state. Bản mới là nơi lưu ticket, có người dùng, có phân quyền, và PO duyệt được ngay trên web thay vì gõ `omi step <ID> approve` trong terminal.

Thuật ngữ: omi-board gọi mọi đơn vị công việc là **ticket**. "Issue" không còn là tên chung mà chỉ là một type của ticket (xem mục 4).

## 2. Vấn đề cần giải quyết

- **Board hiện tại chỉ đọc:** PO xem được nhưng mọi thao tác (duyệt, tạo task, ghi quyết định) vẫn phải qua CLI hoặc chat với Coordinator.
- **Chỉ một người dùng:** state nằm trong git của PO; không có cách mời người khác xem hoặc làm cùng, không có phân quyền.
- **Hiệu năng kém khi state lớn:** payload `/api/projects/omi` khoảng 9 MB, 26k dòng activity, client bỏ request sau 10 giây. Đọc toàn bộ file Markdown mỗi lần không mở rộng được.
- **Không realtime:** board hiện tại tự fetch lại theo chu kỳ (poll endpoint version rồi tải lại snapshot); thay đổi đến chậm, tốn băng thông và dễ bị timeout.
- **Liên kết GitHub lỏng lẻo:** PR, CI và commit chỉ nối với task qua quy ước tên (`task/TASK-NNN`, `[TASK-NNN]` trong tiêu đề), board không hiển thị trực tiếp.

## 3. Người dùng và phân quyền

| Vai trò | Phạm vi | Được làm |
|---|---|---|
| Admin | toàn hệ thống, đúng 1 người | mời thành viên qua email, tạo/xoá dự án, liên kết repo GitHub, gán vai trò theo dự án, thu hồi quyền |
| PO | theo từng dự án | mọi quyền của Member, cộng: duyệt các PO gate (approve), nghiệm thu, ghi quyết định, đổi ưu tiên, đóng/mở lại ticket |
| Member | theo từng dự án | xem, tạo và sửa ticket, bình luận; không duyệt PO gate |
| Viewer | theo từng dự án | chỉ xem ticket, board, báo cáo; không sửa, không comment |
| Squad (máy, không phải người) | theo từng dự án, bằng API token | đọc ticket, ghi step, report, usage, checkpoint; **không bao giờ approve** |

- Không có trang đăng ký. Chỉ Admin mời bằng email; người nhận mở link mời (một lần, có hạn) để đặt mật khẩu hoặc đăng nhập.
- Một người có thể là PO ở dự án này và Member ở dự án khác.
- Token của squad tách khỏi tài khoản người, giống quy tắc hiện tại: squad merge vào development, chỉ PO merge staging/main (ruleset `omi-po-only`). Lời duyệt luôn gắn với một người PO có thật cùng nguyên văn lời họ.

## 4. Ticket

### Type
Giữ đủ 11 type của omi hiện tại (`omi-skill-issue-types`), đổi tên hiển thị cho dễ đọc:

| Type omi-board | Type omi | Ý nghĩa ngắn |
|---|---|---|
| Milestone | milestone | mốc release hoặc ngày PO cam kết; không chạy |
| Epic | epic | kết quả lớn hơn một phiên, chia ticket con; không chạy |
| Feature (FR) | fr | một hành vi người dùng thấy được |
| Quality (NFR) | nfr | mục tiêu chất lượng có con số |
| DevTask | task | việc một phiên không thuộc type khác |
| Bug | bug | hành vi lệch spec hoặc lệch tiêu chí đã nghiệm thu |
| Security | security | lỗ hổng, lộ secret, sai quyền |
| Incident | incident | dịch vụ đang chạy bị hỏng hoặc suy giảm |
| Change | change | PO đổi yêu cầu đã nghiệm thu |
| Spike | spike | câu hỏi cần trả lời trước; đầu ra là khuyến nghị |
| Chore | chore | refactor, nâng dependency, CI, không đổi hành vi |
| Issue | (mới) | vấn đề người dùng báo, chưa phân loại; triage xong thì đổi sang type khác |

### Cây ticket
- Ticket có nhiều ticket con, tạo thành cây như cây issue của board hiện tại. Luật cha–con lấy bảng `tasks.tree_errors` (ví dụ: Epic chỉ nằm dưới Milestone hoặc ở gốc), trừ một điểm: **chỉ DevTask là lá bắt buộc**. Bug, Security, Chore, Change, Incident được có DevTask con (xem mục "Mọi PR gắn với một DevTask" bên dưới). Server từ chối cây sai luật.
- Ticket có con thì trạng thái suy ra từ các con (done/blocked/planned/in-progress), không tự chạy.
- `depends` là quan hệ riêng, không phải quan hệ cha–con; dùng cho đồ thị phụ thuộc.

### Mọi PR gắn với một DevTask (PO chốt 2026-11-07)
- **DevTask là type duy nhất có PR và là type duy nhất squad chạy để sửa code.** Mọi thay đổi code đi qua một DevTask, và DevTask phải liên kết tới PR trên GitHub.
- Các type khác không tự có PR. Bug, Security, Incident, Change, Chore, Feature, Quality, Epic và Milestone mô tả vấn đề hoặc kết quả; phần sửa code nằm ở **ticket con type DevTask**. Ví dụ: Bug "Đăng nhập lỗi 500" chứa Symptom, Expected, Repro; DevTask con "Fix null session in login handler" có PR `[DT-123] fix(auth): ...`.
- Spike không bắt buộc có DevTask; chỉ khi cần prototype code mới tạo DevTask con. Issue phải triage sang type khác trước khi có DevTask con.
- DevTask chạy workflow theo type của cha: cha là Bug thì dùng `omi-flow-bugfix`, Security thì `omi-flow-feature-sensitive`, Chore thì `omi-flow-refactor`, Feature thì `omi-flow-feature` hoặc `omi-flow-feature-ui`. Trường riêng của type cha (Symptom, Impact, Why now...) được đưa vào brief của DevTask.
- Liên kết PR: nhánh `task/<DevTask ID>` và tiêu đề PR bắt đầu bằng `[<DevTask ID>]`. Webhook GitHub tự gắn PR vào DevTask đó, kèm trạng thái draft/ready, CI và commit merge. Một DevTask có đúng một PR đang mở; PR đóng không merge vẫn được giữ trong lịch sử.
- Luật server:
  - DevTask chỉ chuyển sang done khi có PR đã merge.
  - PR tham chiếu tới ticket không phải DevTask, hoặc không tham chiếu ticket nào, bị đánh dấu cảnh báo trên board và nhận comment trên PR.
  - Ticket cha hiện danh sách PR gộp từ mọi DevTask con, và chỉ done khi mọi DevTask con đã done.
- **Thay đổi ở omi:** luật cây hiện tại coi bug, security, chore là lá và chạy trực tiếp. Với backend `board`, các type đó trở thành ticket cha của DevTask, và chỉ DevTask được `omi run`/`omi advance`. Đây là việc phía omi, cần một task riêng trong repo omi.

### Cấu trúc theo type
Trường chung (từ `.omi/tasks/_template.md`): id, title, type, parent, status, depends, plan, scope, role, skills, refs, tool, model, effort, step_settings, workflow, step, Goal, Acceptance criteria (checklist), Checkpoint, Steps (nhật ký bước), Reports (báo cáo của từng agent), Usage (token/chi phí từng lần chạy).

Trường riêng theo type (từ bước 4 của `omi-skill-issue-types`), hiện thành form riêng:
- Bug: Symptom, Expected, Steps to reproduce, Evidence, Source.
- Security: Impact, Boundary crossed, How verified, Evidence (secret đã che).
- Incident: Started at, Impact, Current state, Timeline.
- Change: What changes, lời PO, ticket bị ảnh hưởng, xử lý việc đang chạy.
- Spike: Question, Time-box, Output.
- Chore: Why now, tiêu chí "không đổi hành vi".
- Milestone, Epic: kết quả mong muốn, ngày hoặc điều kiện kết thúc.

Trường do Coordinator quản lý (tool/model/effort, step_settings, plan, refs) chỉ squad hoặc PO sửa được.

## 5. Tính năng v1 (kế thừa board v2)

**Realtime là yêu cầu bắt buộc, không phải tính năng phụ.** Mọi thay đổi (ticket tạo/sửa, step, report, approval, PR/CI từ GitHub, thông báo inbox) được server đẩy tới mọi client đang mở ngay khi ghi xong, không có vòng fetch định kỳ:
- Postgres `LISTEN/NOTIFY` (hoặc outbox table) → backend Go → client qua WebSocket. Sự kiện chỉ chứa phần thay đổi (delta theo ticket), client cập nhật tại chỗ, không tải lại cả dự án.
- Mỗi sự kiện có số thứ tự tăng dần theo dự án; client mất kết nối thì nối lại và xin các sự kiện sau số cuối đã nhận, không mất sự kiện và không phải reload.
- Chỉ đẩy sự kiện của dự án mà người dùng có quyền xem.
- omi CLI dùng cùng luồng sự kiện để nhận approval của PO (xem mục 7).

| Nhóm | Tính năng |
|---|---|
| Dự án | danh sách dự án, mỗi dự án một repo GitHub; plan (PLAN-NNN) và mức hoàn thành |
| Ticket | cây thu gọn/mở rộng, Kanban theo trạng thái, trang chi tiết đầy đủ (criteria, checkpoint, steps, reports, usage), tạo/sửa theo form của từng type |
| Workflow | sơ đồ workflow cùng bước hiện tại; danh mục skills, agents, workflows, rules, references |
| PO inbox | việc đang chờ PO (PO gate, quyết định, `ROUND LIMIT`, `QUOTA`), thông báo khi có việc mới, không báo trùng |
| Duyệt | PO bấm Approve hoặc Reject trên gate, nhập lời duyệt; omi ghi `PO approval <step>: <lời PO>` như hiện nay |
| Theo dõi | cảnh báo việc bị kẹt, đồ thị phụ thuộc có critical path, timeline thời gian từng bước và file đã đổi, chi phí và quota theo tool/model |
| GitHub | mỗi DevTask hiện PR, trạng thái CI, commit merge (liên kết qua nhánh `task/<ID>` và tiêu đề `[<ID>]`); ticket cha hiện PR gộp từ DevTask con; cảnh báo PR không gắn DevTask |
| Báo cáo | báo cáo tuần dạng Markdown tải về được |
| Giao diện | lọc, tìm kiếm, link chia sẻ; sidebar thu gọn, header cố định; sáng/tối; chạy được trên điện thoại (320px); truy cập bằng bàn phím |
| Người dùng | Admin mời qua email, phân quyền PO/Member theo dự án, token squad theo dự án |
| Nhập dữ liệu | nhập toàn bộ task, plan, decision hiện có từ omi-state (dự án `omi` có 236 task) |

### Triển khai prod (PO chốt 2026-11-07)
Prod chạy trên dịch vụ cloud, không chạy bằng Docker trên máy PO. Sau này có thể chuyển sang homelab chạy Docker. Docker local chỉ dùng cho dev và test.

| Thành phần | Prod v1 | Homelab sau này |
|---|---|---|
| PostgreSQL | Supabase (chỉ dùng như Postgres được quản lý) | container `postgres` |
| Backend Go (REST + WebSocket) | Northflank, chạy container từ `Dockerfile` của repo | cùng image đó |
| Frontend Next.js | Netlify | container Node (Next.js `output: standalone`) |

Ràng buộc để chuyển được giữa hai nơi mà không sửa code:
- **Không khoá vào nền tảng:** chỉ dùng Supabase như Postgres thuần, không dùng Supabase Auth, Realtime, Storage hay Edge Functions; auth và realtime do backend Go làm. Không dùng Netlify Functions, Edge hay Blobs; Next.js không có backend riêng.
- **Cấu hình chỉ qua biến môi trường:** `DATABASE_URL`, URL công khai của API và web, secret phiên, khoá GitHub App, cấu hình email. Mọi chỗ dùng chung một bộ biến; có `.env.example` và không commit secret.
- **Realtime trên Supabase:** `LISTEN/NOTIFY` cần kết nối trực tiếp hoặc session pooler (cổng 5432). Transaction pooler (6543) không giữ được `LISTEN`. Backend dùng một kết nối riêng cho listener và pool thường cho truy vấn.
- **WebSocket đi thẳng tới Go trên Northflank,** không qua Netlify (Netlify không giữ kết nối lâu). Frontend và API dùng chung tên miền gốc (vd. `board.<domain>` và `api.<domain>`) để cookie phiên, CORS và WebSocket hoạt động; khi lên homelab thì một reverse proxy làm việc tương tự.
- **Chạy nhiều instance:** backend có thể chạy nhiều replica trên Northflank. Mỗi instance tự `LISTEN` để mọi client nhận đủ sự kiện; không giữ state trong bộ nhớ ngoài kết nối WebSocket.
- **Migration DB:** file SQL có đánh số, backend tự chạy khi khởi động hoặc qua một lệnh riêng. Bản cũ và bản mới chạy được song song trong lúc deploy.
- **`docker-compose.yml`** dựng cả ba thành phần cho dev và làm mẫu cho homelab. Health check `/healthz` cho Northflank và compose.
- **Deploy:** Netlify và Northflank tự deploy từ nhánh `main` (prod) và `staging` (UAT, nếu PO bật). Merge vào staging/main vẫn là quyền của PO. Squad và worker không cần, và không được có, credential của Supabase, Netlify hay Northflank.

### Bổ sung v1 (PO chọn 2026-11-07)
Nguyên tắc: **ưu tiên dịch vụ và gói miễn phí trước**; chỉ dùng gói trả phí khi PO duyệt (chi phí vận hành là quyền của PO).

Bảo mật và dữ liệu
- **Audit log, approval bất biến:** mọi thay đổi ticket ghi lại ai, lúc nào, trước và sau (người, squad token hay webhook GitHub). Bản ghi approval của PO chỉ được thêm, không sửa, không xoá; chứa người duyệt, nguyên văn lời duyệt, ticket, step và thời điểm. Lịch sử hiện trên trang ticket.
- **2FA/passkey cho Admin và PO:** bắt buộc TOTP hoặc passkey (WebAuthn) với Admin và mọi người có vai trò PO; Member tuỳ chọn. Thao tác approve cần phiên đã xác thực 2FA. Token squad theo dự án có ngày hết hạn, xoay vòng được, Admin thu hồi được, chỉ lưu dạng hash.
- **Khoá lạc quan và idempotency:** mỗi ticket có `version`; ghi với version cũ thì nhận 409 và client tải lại phần thay đổi. Mọi request ghi từ omi mang `Idempotency-Key`, nên retry không tạo bản ghi trùng.

Trải nghiệm PO và thành viên
- **Duyệt trên điện thoại:** web là PWA cài được; Web Push (VAPID, miễn phí, không qua dịch vụ trả phí) báo khi có gate chờ PO, ROUND LIMIT hoặc QUOTA. Trang approve dùng tốt ở 320px.
- **Comment và @mention:** comment trong ticket, @mention gửi thông báo trong app và qua push. Comment của PO được Coordinator đọc như lời PO; comment của Member không thay được approval.
- **Priority, assignee, due date:** priority (P0–P3) để `omi dispatch next` chọn task sẵn sàng có priority cao trước; assignee là người hoặc "squad"; due date có cảnh báo quá hạn trong health alerts.
- **Phím tắt:** điều hướng và thao tác chính bằng bàn phím (vd. `/` tìm kiếm, `c` tạo ticket, `j`/`k` lên xuống danh sách, `a` approve có hộp xác nhận); `?` mở bảng phím tắt. Không chặn phím của trình duyệt hay trình đọc màn hình.
- **Viewer (chỉ đọc):** vai trò thứ tư theo dự án, cũng chỉ vào được bằng lời mời của Admin. Viewer xem ticket, board và báo cáo; không tạo, sửa hay comment, không thấy token squad và trang admin.
- **Đính kèm ảnh/file:** lưu trên kho S3-compatible (Cloudflare R2 gói free khi prod, MinIO ở homelab), không dùng Supabase Storage. Giới hạn kích thước và loại file; đính kèm không bao giờ vào worker.

Kỹ thuật và vận hành
- **OpenAPI là hợp đồng:** file `api/openapi.yaml` là nguồn chuẩn, API có version `/v1`. Từ đó sinh code server Go, client TypeScript cho Next.js và client Python cho omi. CI báo lỗi khi code lệch spec. Sự kiện WebSocket cũng được mô tả (schema JSON) trong cùng thư mục.
- **Epic phía omi (trong repo omi, không phải omi-board):** gồm backend state `board` trong `omi.state`; chỉ DevTask được `omi run`/`omi advance`; `omi watch` nghe WebSocket để nhận approval; lệnh nhập omi-state lên board; priority cho `dispatch next`. Các task này lập trong state của dự án omi, song song với omi-board.
- **Giám sát:** log JSON có cấu trúc, mã request; theo dõi lỗi bằng Sentry gói free (backend và frontend); kiểm tra uptime bằng dịch vụ free (vd. UptimeRobot) gọi `/healthz`; số kết nối WebSocket và độ trễ sự kiện hiện trên trang admin.
- **Tìm kiếm full-text:** Postgres full-text search (`tsvector`, có index GIN) trên tiêu đề, Goal, criteria, comment và report; lọc theo type, trạng thái, priority, assignee; kết quả chỉ gồm dự án người dùng được xem. Không dùng dịch vụ search ngoài.
- **Giới hạn tần suất (rate limit):** giới hạn theo người dùng, theo token squad và theo IP; chặt hơn với đăng nhập, link mời, 2FA và approve. Vượt giới hạn trả 429 kèm `Retry-After`; omi tôn trọng `Retry-After` khi retry.
- **Release view và changelog tự sinh:** Milestone gắn với tag `rc-*`/`v*`; changelog (tiếng Việt, theo plan và type) sinh từ PR đã merge của các DevTask trong range, để `omi-flow-release` dùng thay phần ghi chú viết tay.

## 6. Ngoài phạm vi v1

- omi-board không chạy agent, không chạy Docker, không đụng code. Việc thực thi vẫn do omi trên máy PO.
- Không thay GitHub PR/review; board chỉ hiển thị và liên kết.
- Không có đăng ký công khai, thanh toán hay nhiều tổ chức.
- Không có MCP server (xem mục 7); để dành cho sau v1.
- Backup tự động ra kho riêng và export dự án ra định dạng omi-state: PO chưa chọn cho v1. v1 dựa vào backup sẵn có của Supabase; xem lại trước khi dùng thật.

## 7. Giao tiếp giữa omi và omi-board — phân tích và đề xuất

### Ràng buộc từ omi hiện tại
1. **Quota là ưu tiên số một.** Mỗi token đưa vào ngữ cảnh agent đều tốn tiền. Hiện tại agent chỉ thấy vài dòng output CLI đã định dạng sẵn.
2. **Gate phải chạy bằng code, không dựa vào lời agent.** `omi step`, `omi advance`, `omi accept` kiểm tra gate và vòng sửa bằng Python. Nếu agent ghi thẳng vào board thì có thể vượt gate.
3. **Engineer chạy trong Docker sau egress proxy có allowlist** và không cần biết board tồn tại. Chỉ Coordinator trên host (qua CLI `omi`) cần nói chuyện với board.
4. **omi chạy trên máy PO, sau NAT**, nên board không gọi ngược vào omi được (webhook vào máy PO không khả thi).
5. **Đổi được tool giữa các phiên** (Claude Code ↔ Codex); kênh giao tiếp không được gắn với một tool.

### So sánh

| Tiêu chí | REST API (board) + omi CLI làm client | MCP server cho agent | Chỉ CLI riêng của board | Git (omi-state) làm kênh, như hiện nay |
|---|---|---|---|---|
| Chi phí token | thấp: agent vẫn gọi `python -m omi ...`, output vài dòng | cao: schema các tool nạp vào ngữ cảnh mỗi phiên, kết quả JSON dài | thấp | thấp |
| Giữ gate bằng code | có: mọi thao tác ghi đi qua omi rồi mới tới API | yếu: agent gọi tool ghi trực tiếp, phải dựng lại gate phía server | có | có |
| Claude Code và Codex | như nhau (cùng CLI) | cả hai hỗ trợ MCP, nhưng cấu hình riêng cho từng tool | như nhau | như nhau |
| Nhiều người dùng, phân quyền | có (token theo người và theo squad) | có | phụ thuộc API phía sau | không (quyền git là quyền repo) |
| Thời gian thực, PO duyệt trên web | omi giữ kết nối WebSocket ra board, nhận approval ngay lập tức | tương tự | tương tự | chậm (commit/pull), dễ conflict |
| Hiệu năng | truy vấn theo trang, theo ticket | như API | như API | đọc toàn bộ file (9 MB) |
| Việc phải làm ở omi | thay lớp `omi.state` bằng client API | thêm server MCP và cấu hình cho từng tool | thêm một CLI nữa | không |

### Đề xuất
1. **omi-board cung cấp REST API (JSON, HTTPS) và là nơi lưu ticket duy nhất.** Đây là hợp đồng chính; giao diện web cũng dùng chính API này.
2. **omi CLI là client duy nhất của squad.** Lớp state của omi (`omi.state`, hiện đọc/ghi file trong omi-state) có thêm backend "board". Lệnh `omi step`, `omi note`, `omi run`, `omi advance`... giữ nguyên, nên agent, skill và workflow không đổi và không tốn thêm token. Gate vẫn kiểm bằng code ở omi; server kiểm lại quyền (token squad không gọi được endpoint approve).
3. **Duyệt từ web theo thời gian thực:** PO bấm Approve, board ghi sự kiện và đẩy ngay qua luồng sự kiện. `omi watch --wait` mở kết nối WebSocket **từ máy PO ra board** (nên không cần mở cổng vào máy PO, NAT không ảnh hưởng), nhận approval tức thì rồi tiếp tục. Nối lại thì xin bù các sự kiện bị lỡ theo số thứ tự. Lệnh ngắn như `omi status` chỉ gọi REST.
4. **GitHub nói chuyện trực tiếp với board** qua GitHub App/webhook (PR, CI, merge), vì board có địa chỉ công khai. omi không phải chuyển tiếp.
5. **MCP để sau v1, chỉ đọc,** dành cho PO hỏi trong Claude Desktop ("inbox của tôi có gì?"). Không cho agent squad ghi qua MCP.
6. **Chuyển đổi dần:** có lệnh nhập dữ liệu từ omi-state; trong thời gian chuyển, omi chọn backend theo từng dự án (`file` hoặc `board`). Dự án chưa chuyển vẫn chạy như cũ.

Rủi ro chính: omi phụ thuộc mạng và board phải luôn chạy. Cần quyết định omi xử lý thế nào khi board không truy cập được (xem câu hỏi mở 4).

## 8. Câu hỏi mở cho `omi-idea` (kèm đề xuất mặc định)

1. **Stack (PO đã chốt 2026-11-07):** backend **Go**, frontend **Next.js**, database **PostgreSQL**, đều dùng bản stable mới nhất. TASK-001 tra version stable mới nhất tại thời điểm làm rồi ghim chính xác (`go.mod` toolchain, `package.json` không dùng `^`/`latest`, image `postgres:<major>`), không ghi "latest" trong file. Go phục vụ REST API và WebSocket. Next.js chỉ là frontend, gọi API Go và nhận sự kiện qua WebSocket, không có backend riêng. Dev và CI dùng Postgres thật trong Docker, không thay bằng SQLite. `network_allow` của worker: proxy.golang.org, sum.golang.org, storage.googleapis.com, registry.npmjs.org; image worker cần Go, Node và Postgres client (test cần một Postgres chạy được trong worker hoặc service container ở CI).
2. **Hosting (đã chốt: Supabase, Netlify, Northflank; xem "Triển khai prod" ở mục 5).** Còn hỏi: gói nào (đề xuất gói miễn phí cho cả ba lúc đầu; Supabase free tạm dừng project sau 7 ngày không hoạt động, nên chọn gói trả phí trước khi dùng thật), tên miền nào, và có bật môi trường staging riêng không (đề xuất có: một project Supabase và một service Northflank cho staging).
3. **Gửi email mời:** đề xuất dịch vụ có gói miễn phí (Resend hoặc SMTP của Google Workspace sẵn có); chưa cấu hình email thì Admin copy link mời thủ công.
4. **Khi board không truy cập được:** đề xuất omi ghi đệm thay đổi vào hàng đợi local và đồng bộ lại sau; lệnh cần quyết định của PO thì chờ.
5. **Đăng nhập:** đề xuất email + mật khẩu và magic link; đăng nhập bằng GitHub OAuth để sau.
6. **Type "Issue":** hiểu là ticket chưa phân loại, phải triage sang type khác trước khi chạy. Đúng ý PO không?
7. **Liên kết GitHub:** đề xuất GitHub App cài vào org `omi-teams` (đọc PR, checks, nhận webhook), không dùng PAT cá nhân.
8. **Ngôn ngữ giao diện:** đề xuất tiếng Việt cho giao diện, giữ thuật ngữ kỹ thuật tiếng Anh (type, workflow, step), code và API bằng tiếng Anh.
9. **Dữ liệu cũ:** đề xuất nhập toàn bộ omi-state của dự án `omi` làm dữ liệu thật để kiểm thử hiệu năng (236 task, hàng chục nghìn dòng activity).
10. **Mã ticket:** đề xuất mã theo dự án, một dãy số chung cho mọi type (ví dụ `BOARD-42`), để nhánh `task/BOARD-42` và tiêu đề `[BOARD-42]` không đổi khi đổi type. `DT-123` trong mục 4 chỉ là ví dụ. Dữ liệu nhập từ omi-state giữ nguyên `TASK-NNN`.
11. **Repo:** `omi-teams/omi-board` đang public (cần cho rulesets ở gói Free). Không commit secret; cấu hình qua biến môi trường.

## 9. Tiêu chí thành công v1

- PO duyệt một PO gate trên web, rồi `omi watch` trên máy PO tiếp tục task đó trong vòng 2 giây mà không cần gõ `omi step approve`.
- Hai trình duyệt mở cùng dự án: thay đổi ở bên này (hoặc từ omi CLI, hoặc webhook GitHub) hiện ở bên kia trong vòng 1 giây, không reload trang. Trong tab Network không có request fetch định kỳ. Ngắt mạng 30 giây rồi nối lại: client nhận bù đủ sự kiện đã lỡ.
- Một task thật của dự án omi chạy từ đầu đến merge với backend `board`; agent không đổi prompt và tốn không quá số token của backend `file` hiện nay.
- Admin mời một Member qua email; Member tạo được ticket nhưng không approve được (server trả 403); token squad cũng không approve được.
- Trang dự án `omi` sau khi nhập dữ liệu mở trong dưới 2 giây; trang chi tiết ticket dưới 1 giây.
- Cây ticket từ chối quan hệ cha–con sai luật (luật của `tasks.tree_errors`, có thêm: DevTask là lá và được nằm dưới Bug, Security, Chore, Change, Incident).
- Mỗi DevTask hiện PR, trạng thái CI và commit merge lấy từ GitHub; Bug có DevTask con hiện PR của con.
- DevTask không có PR đã merge thì không chuyển sang done được (server từ chối). Một PR có tiêu đề trỏ tới Bug (không phải DevTask) bị cảnh báo trên board và có comment trên PR.
