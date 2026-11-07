# omi-board — Product spec v1

Duyệt: 2026-11-07. Chi tiết đầy đủ ở [project-overview.md](../project-overview.md); file này là bản chốt.

## Người dùng
- **Admin** (đúng 1 người): mời thành viên qua email, tạo/xoá dự án, liên kết repo, gán vai trò, thu hồi quyền.
- **PO** (theo dự án): quyền của Member, cộng duyệt PO gate, nghiệm thu, ghi quyết định, đổi ưu tiên, đóng/mở lại ticket.
- **Member** (theo dự án): xem, tạo, sửa ticket, comment; không approve.
- **Viewer** (theo dự án): chỉ xem.
- **Squad** (máy, token theo dự án): đọc ticket, ghi step/report/usage/checkpoint; không bao giờ approve.

Không có trang đăng ký; chỉ Admin mời qua email.

## Vấn đề
Board hiện tại chỉ cho đọc, chỉ một người dùng, chậm khi state lớn (payload 9 MB, timeout sau 10 giây), không realtime, và liên kết với GitHub lỏng lẻo. PO vẫn phải duyệt bằng CLI.

## Tính năng v1
Làm đủ toàn bộ rồi mới dùng thật (PO chốt).
- **Ticket:** 12 type (11 type của omi + Issue chưa phân loại). Ticket xếp thành cây, luật cây do server kiểm (theo `tasks.tree_errors`; chỉ DevTask là lá bắt buộc). `depends` là quan hệ riêng. Mỗi type có form riêng. Mã ticket theo dự án (`BOARD-42`); dữ liệu nhập giữ `TASK-NNN`.
- **DevTask là type duy nhất có PR.** PR liên kết qua nhánh `task/<ID>` và tiêu đề `[<ID>]`. DevTask chỉ done khi PR đã merge. PR không gắn DevTask bị cảnh báo và nhận comment.
- **Realtime bắt buộc:** Postgres LISTEN/NOTIFY, backend Go, WebSocket tới client. Sự kiện là delta, đánh số thứ tự theo dự án, nối lại thì nhận bù. Không có poll.
- **Giao diện:** cây, Kanban, trang chi tiết (criteria, checkpoint, steps, reports, usage), sơ đồ workflow, danh mục skills/agents/rules, PO inbox, Approve/Reject trên web, cảnh báo việc kẹt, đồ thị phụ thuộc và critical path, timeline, chi phí/quota, báo cáo tuần Markdown, lọc/tìm/link chia sẻ, sáng/tối, rộng tối thiểu 320px, phím tắt.
- **GitHub App** cài vào org `omi-teams`: PR, CI, commit merge qua webhook.
- **Bảo mật:** audit log; bản ghi approval chỉ được thêm. Admin và PO bắt buộc TOTP/passkey. Token squad có hạn, xoay vòng được, lưu dạng hash. Khoá lạc quan (`version`, trả 409), `Idempotency-Key`, rate limit (trả 429 kèm `Retry-After`).
- **Khôi phục Admin:** 10 recovery code dùng một lần. Thêm lệnh CLI trên server (cần `DATABASE_URL`) để reset 2FA của Admin.
- **Dữ liệu lớn:** steps và activity phân trang theo con trỏ. Sự kiện WebSocket cũ hơn 90 ngày được archive (client nối lại sau thời gian dài thì tải lại snapshot).
- **Khác:** PWA và Web Push (VAPID); comment và @mention; priority P0–P3, assignee, due date; đính kèm trên S3-compatible (R2/MinIO); full-text search Postgres; release view và changelog tự sinh; Sentry, UptimeRobot, chỉ số WebSocket trên trang admin.
- **API:** `api/openapi.yaml` là hợp đồng `/v1`. Từ đó sinh server Go, client TypeScript và client Python. CI báo lỗi khi code lệch spec.
- **Nhập dữ liệu** từ omi-state (dự án `omi`, 236 task).
- **Đăng nhập** bằng email + mật khẩu và magic link. Email gửi qua Resend gói free; chưa cấu hình thì Admin copy link mời thủ công.
- Giao diện tiếng Việt; thuật ngữ kỹ thuật, code và API tiếng Anh.

## Ngoài phạm vi v1
- Board không chạy agent, Docker hay code; việc thực thi vẫn do omi trên máy PO.
- Không thay GitHub PR/review.
- Không có đăng ký công khai, thanh toán hay nhiều tổ chức.
- Không có MCP server.
- Không có GitHub OAuth.
- Không có backup riêng hay export ra omi-state. Rủi ro: Supabase gói free không có backup để khôi phục, nên phải nâng gói trước khi dùng thật.
- **Việc phía repo omi** (backend `board` trong `omi.state`, `omi watch` qua WebSocket, hàng đợi offline, priority cho `dispatch next`, chỉ DevTask được `run`/`advance`) lập task trong state của dự án omi, không thuộc repo này.

## Ràng buộc
- **Hosting prod:** Supabase (chỉ dùng như Postgres thuần), Northflank (Go container), Netlify (Next.js tĩnh/SSR, không dùng Functions/Edge/Blobs). Có staging riêng. Dùng gói free lúc phát triển; nâng Supabase lên gói trả phí trước khi dùng thật, cần PO duyệt.
- **Tên miền:** PO mua sau, dạng `board.<domain>` và `api.<domain>`. Tạm dùng subdomain mặc định của dịch vụ.
- **Di chuyển được sang homelab** mà không sửa code: cấu hình chỉ qua biến môi trường (có `.env.example`), có `docker-compose.yml` và `/healthz`. Backend chạy nhiều replica được. Migration SQL đánh số, bản cũ và mới chạy song song được.
- **LISTEN** dùng kết nối trực tiếp hoặc session pooler (cổng 5432). WebSocket đi thẳng tới Go.
- **Phía board hỗ trợ omi chạy offline:** idempotency và số thứ tự sự kiện, để omi đệm thao tác ghi rồi đồng bộ lại sau.
- **Repo public.** Không commit secret. Squad không có credential cloud.
- **Ghim version stable chính xác:** `go.mod` toolchain, `package.json` không dùng `^`, `postgres:<major>`.

## Stack
| Thành phần | Chọn | Lý do |
|---|---|---|
| Backend | Go (REST + WebSocket) | PO chốt; một binary nhẹ, giữ nhiều kết nối WebSocket tốt |
| Frontend | Next.js (chỉ frontend) | PO chốt; deploy được trên Netlify và dạng `standalone` ở homelab |
| Database | PostgreSQL | LISTEN/NOTIFY cho realtime, full-text search sẵn có, Supabase và homelab như nhau |
| Hợp đồng API | OpenAPI 3 + codegen | một nguồn chuẩn cho Go, TypeScript và Python (omi) |
| File đính kèm | S3-compatible (R2 / MinIO) | gói free, không khoá vào Supabase |
| Dev/CI | Docker Compose, Postgres thật | giống prod, không dùng SQLite |
| `network_allow` | proxy.golang.org, sum.golang.org, storage.googleapis.com, registry.npmjs.org | toolchain Go và npm |

## Tiêu chí thành công
1. PO approve trên web; `omi watch` tiếp tục task trong ≤ 2 giây.
2. Thay đổi (từ web, CLI hoặc webhook) hiện ở trình duyệt khác trong ≤ 1 giây, không có fetch định kỳ. Mất mạng 30 giây rồi nối lại thì nhận bù đủ sự kiện.
3. Một task thật của omi chạy tới merge với backend `board`, không đổi prompt, không tốn thêm token.
4. Member và token squad gọi approve thì nhận 403.
5. Trang dự án `omi` sau khi nhập dữ liệu mở dưới 2 giây; trang chi tiết ticket dưới 1 giây.
6. Server từ chối cây sai luật và từ chối DevTask done khi chưa có PR đã merge. PR trỏ tới ticket không phải DevTask bị cảnh báo và nhận comment.
7. Mỗi DevTask hiện PR, CI và commit merge; ticket cha hiện PR gộp từ các DevTask con.
8. Giao diện đạt WCAG 2.2 AA: axe trong CI không báo lỗi nào mức serious hoặc critical.
9. Test tải trong CI (dữ liệu omi, 50 kết nối WebSocket): độ trễ sự kiện p95 ≤ 1 giây.
10. Admin mất 2FA vẫn khôi phục được bằng recovery code hoặc lệnh CLI trên server.
