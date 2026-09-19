# Google session gateway: kế hoạch Phase 0–Phase 8

> Ngày lập: 2026-09-18  
> Phạm vi: phát triển gateway nội bộ/public có khả năng giao tiếp với Flow và Gemini thông qua các phiên Google được quản lý trong Chrome.  
> Trạng thái: kế hoạch triển khai, chưa phải xác nhận production-ready.

## Tóm tắt quyết định

Dự án hiện đã chứng minh được các khả năng nền tảng:

- đăng nhập Google bằng Chrome profile riêng;
- lấy cookie/session cần thiết;
- giao tiếp với Flow;
- lấy được Flow credits;
- giao tiếp với Gemini;
- lấy được Gemini quota.

Bước tiếp theo không phải là thêm thật nhiều account rồi round-robin ngay. Cần xây dựng theo thứ tự:

1. ổn định identity và lifecycle của một account;
2. tách provider adapter cho Flow và Gemini;
3. chuẩn hóa quota/credit snapshot;
4. xây dựng job system;
5. xây dựng account lease và scheduler;
6. thêm gateway API authentication và tenant isolation;
7. kiểm thử failure/recovery/E2E;
8. chỉ sau đó mới mở pool thật.

Kế hoạch này không lấy OAuth/API chính thức làm core vì mục tiêu hiện tại là giao tiếp với Flow và Gemini thông qua phiên browser/session. Tuy nhiên, mô hình browser-session có rủi ro vận hành và điều khoản riêng; Flow credits được gắn với account/subscription, còn các trang/API nội bộ có thể thay đổi bất kỳ lúc nào. Cần xem lại [tài liệu Flow credits](https://support.google.com/flow/answer/16526234?hl=en), [Gemini API Terms](https://ai.google.dev/gemini-api/terms) và [Abuse monitoring](https://ai.google.dev/gemini-api/docs/usage-policies) trước khi mở cho người dùng bên ngoài.

Pool trong tài liệu này được hiểu là tập hợp account do hệ thống quản lý hợp lệ và điều phối theo capacity được phép. Không thiết kế cơ chế chuyển account tức thời chỉ để vượt 429, quota hoặc anti-abuse.

## Phạm vi và ranh giới thay đổi

### Trong phạm vi

- login, refresh, verify và revoke browser session;
- lấy identity, Flow credits và Gemini quota;
- quản lý nhiều account;
- điều phối request/job qua account pool;
- gateway API cho người dùng;
- audit log, quota ledger, job recovery;
- kiểm thử Chrome/CDP/provider adapter;
- xác minh artifact Flow sau khi tạo.

### Ngoài phạm vi

- lưu hoặc xử lý password Google;
- trả cookie, SNlM0e, Chrome profile hoặc proxy credential cho người dùng API;
- tự động xóa profile/account mà chưa có reconciliation và approval;
- dùng account rotation để né quota hoặc chính sách chống lạm dụng;
- tự động commit, push, deploy hoặc release;
- thay toàn bộ core hiện tại trong một lần rewrite.

### Nguyên tắc bảo toàn dữ liệu

- Giữ nguyên source và dữ liệu hiện có trong khi lập kế hoạch.
- Migration phải có backup, kiểm tra trước/sau và rollback strategy.
- Không xóa các profile đang tồn tại chỉ vì chưa được database tham chiếu.
- Các cột legacy chỉ được xóa sau khi migration đã được xác nhận và có thời gian rollback.
- Cookie/token/proxy không xuất hiện trong fixture, log, exception message hoặc tài liệu.

## Baseline hiện tại

| Hạng mục | Trạng thái hiện tại | Đánh giá |
|---|---|---|
| Login Google qua Chrome | Đã có | Cần đưa về state machine và identity verification |
| Lấy Flow credits | Đã có | Cần snapshot có nguồn, thời gian và độ tin cậy |
| Lấy Gemini quota | Đã có | Không được coi dữ liệu quan sát là số liệu tuyệt đối |
| Lưu cookie/token/proxy | AES-GCM, master key Windows DPAPI | Tốt về nền tảng, cần tách credential/session |
| CDP | Loopback port và /json | Cần harden trước khi chạy pool |
| Database | google_accounts đang gộp nhiều vai trò | Cần tách account/session/quota/job |
| Pool scheduler | Chưa có đầy đủ | Cần lease, cooldown, health và recovery |
| Public gateway API | Chưa hoàn chỉnh | Gateway hiện chủ yếu là health/local service |
| Job system Flow | Chưa hoàn chỉnh | Cần async state, polling, cancel và artifact validation |
| E2E với Google/Chrome thật | Chưa chạy trong audit | Phải là opt-in, không đưa credential thật vào CI |
| go test ./... | PASS | Đã kiểm tra |
| go test -race ./... | PASS | Đã kiểm tra |
| Frontend build | PASS | Đã kiểm tra |
| Wails build | PASS | Đã kiểm tra |
| go vet ./... | FAIL | Lệch Go directive/API tại process_windows.go |

Trong snapshot read-only hiện tại, database có một user local và một Google account hoạt động; các trường nhạy cảm trong database có tiền tố mã hóa enc:v1:. Có hai thư mục profile nhưng chỉ một profile được database tham chiếu; cần reconciliation trước khi quyết định xử lý profile còn lại.

## Kiến trúc mục tiêu

~~~mermaid
flowchart TD
    U[API user or internal client] --> A[Gateway API]
    A --> AU[API key and tenant authorization]
    A --> JM[Request and Job Manager]
    JM --> S[Account Pool Scheduler]
    S --> L[Account Lease and cooldown]
    L --> PA[Flow or Gemini Provider Adapter]
    PA --> SM[Session Manager]
    SM --> CDP[Chrome profile and CDP]
    CDP --> G[Google web session]
    PA --> QS[Quota and credit snapshot]
    JM --> UE[Usage events]
    UE --> DB[(SQLite database)]
    QS --> DB
    L --> DB
    AU --> DB
    ADM[Admin Wails UI] --> AD[Admin service boundary]
    AD --> DB
    AD --> SM
    AD --> S
~~~

### Nguyên tắc phân tách

| Thành phần | Trách nhiệm | Không nên làm |
|---|---|---|
| Admin UI | Login local, quản lý account/profile/session | Không chứa cookie/token trong state UI |
| Gateway API | Auth user, nhận request, trả job/stream | Không quyết định trực tiếp Chrome profile |
| Job Manager | Trạng thái, retry, idempotency, recovery | Không tự đọc DOM/provider endpoint |
| Scheduler | Chọn account, lease, cooldown, fairness | Không parse Flow/Gemini HTML |
| Provider Adapter | Giao tiếp Flow/Gemini, chuẩn hóa lỗi | Không quản lý tenant hoặc API key |
| Session Manager | Chrome profile, CDP, refresh, cleanup | Không tính billing/usage user |
| Database | Durable state và audit | Không lưu secret plaintext |

## Mô hình trạng thái

~~~mermaid
stateDiagram-v2
    [*] --> PENDING
    PENDING --> VERIFYING
    VERIFYING --> READY
    VERIFYING --> CHALLENGE_REQUIRED
    VERIFYING --> EXPIRED
    READY --> REFRESHING
    REFRESHING --> READY
    REFRESHING --> EXPIRED
    READY --> THROTTLED
    THROTTLED --> READY
    READY --> DEGRADED
    DEGRADED --> READY
    READY --> DISABLED
    CHALLENGE_REQUIRED --> VERIFYING
    EXPIRED --> VERIFYING
    DISABLED --> [*]
~~~

Không dùng một boolean active=true cho mọi trạng thái. Account, service session, provider capability và job phải có trạng thái độc lập.

## Evidence map và giới hạn audit

Các điểm source chính dùng làm căn cứ cho kế hoạch:

| Evidence | Nội dung liên quan |
|---|---|
| [app.go](app.go#L184) | Admin authorization boundary và các wrapper thao tác Google account |
| [login_manager.go](internal/services/chrome/login_manager.go#L67) | Chrome profile, login session, CDP port và Flow/Gemini hydration |
| [cdp_client.go](internal/services/chrome/cdp_client.go#L251) | Lấy cookie, set cookie, target discovery và cookie filtering |
| [account_service.go](internal/services/account_service.go#L429) | Test session, manual import, refresh và account lifecycle |
| [db.go](internal/db/db.go#L125) | Database schema, migration và legacy columns |
| [account_db.go](internal/db/account_db.go#L215) | Create/Upsert/encrypt/decrypt account data |
| [live_metrics.go](internal/services/chrome/live_metrics.go#L17) | Live Flow/Gemini metrics và parser đặc thù provider |
| [keepalive_worker.go](internal/services/chrome/keepalive_worker.go#L174) | Refresh session, quota/tier extraction và process lifecycle |
| [models/account.go](internal/models/account.go#L34) | Public response model và redaction |
| [gateway.go](internal/services/gateway.go#L126) | Local gateway health/server boundary |
| [process_windows.go](internal/services/chrome/process_windows.go#L60) | Windows process handle và go vet mismatch |
| [AccountsPage.tsx](frontend/src/pages/AccountsPage.tsx#L92) | UI live metrics, login polling và account actions |

Giới hạn của audit baseline:

- chưa chạy login/refresh Flow/Gemini bằng credential thật trong CI;
- chưa kiểm tra tải lớn lên provider thật;
- chưa có E2E đầy đủ cho Chrome lifecycle, lease recovery và artifact job;
- quota/credit hiện mới là dữ liệu quan sát từ session/provider, chưa có ledger authoritative;
- provider có thể thay đổi DOM, endpoint, challenge hoặc session behavior ngoài phạm vi source hiện tại.

## Bản đồ thay đổi source

### Các file cần sửa hoặc tái cấu trúc

| File hiện tại | Thay đổi dự kiến |
|---|---|
| app.go | Giữ Wails façade; chuyển business logic account/pool/job sang service mới; tất cả wrapper vẫn phải qua admin authorization |
| internal/services/auth.go | Chuẩn hóa password validator, session expiry/inactivity và invalidate session sau khi đổi password |
| internal/services/account_service.go | Giữ backward-compatible façade trong giai đoạn migration; tách identity, session, quota, job và scheduler |
| internal/services/chrome/login_manager.go | Tách thành Chrome/session lifecycle manager; không chứa logic quota/Flow/Gemini |
| internal/services/chrome/cdp_client.go | Giới hạn cookie theo domain/path/name; harden CDP; không ghép cookie cross-domain tùy tiện |
| internal/services/chrome/keepalive_worker.go | Chuyển refresh/health check vào session manager và provider adapter |
| internal/services/chrome/live_metrics.go | Chuyển parser Flow/Gemini vào adapter riêng; trả về snapshot chuẩn hóa |
| internal/services/chrome/process_windows.go | Đồng bộ Go directive/API và kiểm tra process job/cleanup |
| internal/db/db.go | Thêm migration versioning, migration lock và bảng mới |
| internal/db/account_db.go | Tách persistence account/session/quota/lease/job; vẫn giữ decrypt/encrypt tập trung |
| internal/models/account.go | Tách internal model khỏi public response DTO; không expose secret |
| internal/services/gateway.go | Giữ health/readiness và transport; không nhúng scheduler/provider business logic |
| frontend/src/pages/AccountsPage.tsx | Hiển thị session state, provider health, quota snapshot và cảnh báo stale |
| frontend/src/components/accounts/* | Tách rõ login, manual import, refresh, challenge và revoke; không hiển thị secret |
| README.md | Cập nhật kiến trúc thật, prerequisite Go, giới hạn browser-session, test gates và không tuyên bố polling nếu code chưa có |

### Các file/module cần viết mới

Đây là cấu trúc đề xuất; giữ tên phù hợp với convention thực tế của repository:

~~~text
internal/domain/provider_account.go
internal/domain/provider_session.go
internal/domain/quota_snapshot.go
internal/domain/account_lease.go
internal/domain/gateway_job.go
internal/domain/usage_event.go

internal/providers/provider.go
internal/providers/google/session_adapter.go
internal/providers/google/flow_adapter.go
internal/providers/google/gemini_adapter.go
internal/providers/google/parsers/

internal/session/session_manager.go
internal/session/state_machine.go
internal/session/identity_verifier.go

internal/scheduler/pool_scheduler.go
internal/scheduler/lease_manager.go
internal/scheduler/cooldown.go
internal/scheduler/circuit_breaker.go

internal/jobs/job_manager.go
internal/jobs/recovery.go
internal/jobs/idempotency.go

internal/api/http_server.go
internal/api/auth.go
internal/api/routes.go
internal/api/tenant.go
internal/api/errors.go

internal/db/migrations/
internal/testfixtures/providers/
internal/integration/
~~~

### Những thứ cần deprecate hoặc xóa sau migration

Không xóa ngay. Chỉ xóa sau khi có migration evidence và rollback window.

| Thành phần | Cách xử lý |
|---|---|
| Cột legacy password, recovery_email | Giữ trong migration đầu, xác nhận luôn rỗng, sau đó drop trong migration riêng |
| services dạng CSV flow,gemini | Thay bằng bảng account_capabilities hoặc bảng service relation |
| credits mơ hồ trong google_accounts | Thay bằng quota_snapshots và usage_events |
| Absolute profile_dir | Chuyển sang profile_id tương đối; chỉ xóa cột cũ sau khi mọi profile đã migrate |
| FilterGoogleCookies gom cookie quá rộng | Thay bằng cookie policy theo service/domain/path, sau đó xóa implementation cũ |
| Parser Flow/Gemini rải trong live_metrics.go | Chuyển hết vào provider adapter rồi xóa parser cũ |
| Logic login/refresh provider trong service tổng | Giữ façade tạm thời, sau đó xóa duplicate path |
| Profile mồ côi | Chỉ xóa sau reconciliation, grace period và xác nhận không còn account/job dùng |
| Plaintext temporary backup/import | Thay cleanup/private temp flow trước khi xóa implementation cũ |

# Phase 0 — Baseline và contract

## Mục tiêu

Đóng băng hành vi hiện tại, ghi nhận bằng chứng và đặt contract trước khi thay đổi kiến trúc.

## Việc cần làm

1. Tạo fixture sanitized cho:
   - login thành công;
   - login cần 2FA;
   - session hết hạn;
   - Flow credits;
   - Gemini quota;
   - provider response lỗi;
   - CAPTCHA/challenge.
2. Ghi rõ mỗi field lấy từ đâu:
   - CDP cookie;
   - DOM;
   - JavaScript;
   - endpoint nội bộ;
   - response header;
   - dữ liệu do người dùng nhập.
3. Định nghĩa schema snapshot:

~~~json
{
  "account_id": "internal-id",
  "service": "flow",
  "metric": "credits_remaining",
  "value": 0,
  "reset_at": null,
  "source": "provider-page",
  "observed_at": "timestamp",
  "confidence": "medium"
}
~~~

4. Định nghĩa status chuẩn:
   - account status;
   - service session status;
   - job status;
   - provider error status.
5. Tạo _progress.md với các cột:

~~~text
Phase | Work item | Status | Evidence | Blocker | Next action
~~~

6. Sửa go vet trước khi bắt đầu migration lớn.
7. Ghi lại working tree trước khi sửa. Không dùng git reset --hard hoặc xóa thay đổi đang có.

## File cần sửa

- README.md
- internal/services/chrome/process_windows.go
- internal/services/chrome/live_metrics.go
- internal/services/chrome/keepalive_worker.go
- internal/services/chrome/login_manager.go

## File cần viết mới

- _progress.md
- internal/testfixtures/providers/flow/*.json
- internal/testfixtures/providers/gemini/*.json
- internal/testfixtures/providers/README.md
- docs/provider-data-map.md

## Gate hoàn thành

- Một account login/refresh thành công tối thiểu 10 chu kỳ.
- Không có secret trong fixture/log.
- Mọi snapshot có source, timestamp và confidence.
- go test, go test -race, frontend build và Wails build vẫn pass.
- go vet không còn lỗi hoặc có lý do/blocker được ghi rõ.

## Stop rules

- Dừng nếu fixture chứa cookie/token thật.
- Dừng nếu provider trả dữ liệu không đủ để phân biệt UNKNOWN với ZERO.
- Dừng nếu login thành công nhưng identity thật chưa xác minh được.

# Phase 1 — Identity và session lifecycle

## Mục tiêu

Biến login hiện tại thành session lifecycle có thể restart, refresh, revoke và xác minh identity an toàn.

## Data model cần thêm

~~~text
provider_accounts
  id
  provider
  provider_subject
  verified_email
  display_name
  status
  verified_at

provider_sessions
  id
  account_id
  service
  credential_ciphertext
  profile_id
  status
  last_verified_at
  expires_at
  last_error
  credential_version
~~~

## Việc cần làm

1. Thêm identity verification sau khi cookie được lấy.
2. Không đánh dấu ACTIVE/READY chỉ vì có SID hoặc HTTP 2xx.
3. So khớp email người dùng nhập với email Google thật.
4. Lưu provider subject/immutable ID nếu provider cho phép quan sát được.
5. Tách session Flow và Gemini.
6. Thêm revoke:
   - xóa session credential;
   - đóng Chrome;
   - giữ account metadata;
   - chuyển trạng thái về PENDING hoặc EXPIRED.
7. Thêm session refresh:
   - không chạy đồng thời hai refresh cùng account/service;
   - giới hạn retry;
   - lưu last_error đã redacted.
8. Thêm challenge handling:
   - CAPTCHA;
   - 2FA;
   - re-authentication;
   - suspicious activity.
9. Thay CDP port không xác thực bằng pipe hoặc port có identity verification.
10. Profile path lưu relative profile_id; derive path từ data root.

## File cần sửa

- internal/services/chrome/login_manager.go
- internal/services/chrome/cdp_client.go
- internal/services/chrome/keepalive_worker.go
- internal/services/account_service.go
- internal/models/account.go
- internal/db/account_db.go
- app.go

## File cần viết mới

- internal/session/session_manager.go
- internal/session/state_machine.go
- internal/session/identity_verifier.go
- internal/session/session_errors.go
- internal/testfixtures/providers/identity/

## Gate hoàn thành

- Email nhập sai so với session thật bị từ chối.
- Session hết hạn chuyển đúng sang EXPIRED.
- Challenge chuyển sang CHALLENGE_REQUIRED, không retry vô hạn.
- Đóng app giữa lúc login không để Chrome mồ côi.
- Restart app vẫn khôi phục được metadata/session state.
- Không một API public nào trả raw cookie/token/profile path.

## Stop rules

- Không mở pool nếu identity chưa xác minh.
- Không lưu session nếu không biết account nào sở hữu session.
- Không auto-retry khi provider yêu cầu người dùng giải CAPTCHA/2FA.

# Phase 2 — Database và persistence

## Mục tiêu

Tách durable state thành account, session, capability, quota, lease, job và usage.

## Schema mục tiêu

~~~sql
provider_accounts(
  id,
  provider,
  provider_subject,
  verified_email,
  display_name,
  plan,
  region,
  status,
  created_at,
  updated_at
)

provider_sessions(
  id,
  account_id,
  service,
  cookies_encrypted,
  token_encrypted,
  profile_id,
  status,
  last_verified_at,
  expires_at,
  last_error
)

account_capabilities(
  account_id,
  service,
  capability,
  enabled,
  observed_at
)

quota_snapshots(
  id,
  account_id,
  service,
  metric,
  value,
  reset_at,
  source,
  confidence,
  observed_at
)

account_leases(
  id,
  account_id,
  service,
  job_id,
  acquired_at,
  heartbeat_at,
  expires_at,
  released_at
)

gateway_jobs(
  id,
  tenant_id,
  service,
  idempotency_key,
  state,
  account_id,
  attempt,
  result_location,
  error_code,
  created_at,
  completed_at
)

usage_events(
  id,
  tenant_id,
  account_id,
  service,
  job_id,
  input_units,
  output_units,
  estimated_cost,
  observed_cost,
  observed_at
)
~~~

## Việc cần làm

1. Thêm migration version table.
2. Mỗi migration phải:
   - idempotent;
   - chạy trong transaction nếu có thể;
   - ghi version;
   - có kiểm tra schema trước/sau.
3. Giữ AES-GCM/DPAPI hiện tại cho credential.
4. Không ghi quota hiện tại đè lên account metadata.
5. Tách last_error theo account/service/job.
6. Thêm unique constraint phù hợp:
   - provider + subject;
   - account + service;
   - job + idempotency key + tenant.
7. Thêm trạng thái hợp lệ bằng constant/validation thống nhất.
8. Kiểm tra WAL/backup/lock trước và sau migration.
9. Tạo reconciliation report:
   - database account không có profile;
   - profile không có database account;
   - profile đang bị lock;
   - session không còn credential.

## File cần sửa

- internal/db/db.go
- internal/db/account_db.go
- internal/models/account.go
- internal/security/storage_acl_windows.go

## File cần viết mới

- internal/db/migrations/001_provider_accounts.sql
- internal/db/migrations/002_provider_sessions.sql
- internal/db/migrations/003_quota_snapshots.sql
- internal/db/migrations/004_jobs_and_leases.sql
- internal/db/migrations/005_usage_events.sql
- internal/db/reconciliation.go
- internal/db/migration_test.go

## Deprecate/xóa sau migration

- password và recovery_email legacy columns;
- services CSV;
- credits không có ngữ nghĩa rõ;
- absolute profile_dir;
- các query update account ghi đè cookie/token không có patch semantics.

## Gate hoàn thành

- Database cũ migrate không mất account.
- Migration chạy lại không tạo duplicate.
- mode=ro query sau migration đọc đúng dữ liệu.
- Secret vẫn có tiền tố mã hóa.
- Reconciliation report không tự động xóa dữ liệu.

# Phase 3 — Provider adapters

## Mục tiêu

Tách logic đặc thù Flow/Gemini khỏi Chrome/session infrastructure.

## Contract đề xuất

~~~go
type ProviderAdapter interface {
    VerifyIdentity(ctx context.Context, session Session) (Identity, error)
    CheckSession(ctx context.Context, session Session) (SessionStatus, error)
    GetCapabilities(ctx context.Context, session Session) (Capabilities, error)
    GetUsage(ctx context.Context, session Session) (UsageSnapshot, error)
    Submit(ctx context.Context, request ProviderRequest) (ProviderJob, error)
    Poll(ctx context.Context, job ProviderJob) (ProviderJobStatus, error)
    Cancel(ctx context.Context, job ProviderJob) error
}
~~~

## Flow adapter

Flow adapter chịu trách nhiệm:

- kiểm tra Flow session;
- lấy Flow/AI credits;
- nhận biết model/capability;
- submit job;
- poll job;
- lấy output artifact;
- nhận biết credit estimate/actual khi có thể;
- trả lỗi CREDIT_EXHAUSTED, CHALLENGE_REQUIRED, PROVIDER_CHANGED.

Không hardcode selector Flow trong scheduler.

## Gemini adapter

Gemini adapter chịu trách nhiệm:

- kiểm tra Gemini session;
- lấy quota snapshot;
- hỗ trợ request thường;
- hỗ trợ streaming nếu khả thi;
- chuẩn hóa token/request quota;
- trả lỗi QUOTA_EXHAUSTED, RATE_LIMITED, SESSION_EXPIRED.

Không hardcode giá trị quota cố định. Quota phải có observed_at, reset_at và nguồn dữ liệu.

## Việc cần làm

1. Di chuyển parser từ live_metrics.go vào adapter.
2. Di chuyển endpoint/DOM selector vào adapter.
3. Thêm parser version.
4. Nếu provider thay đổi HTML/API, chỉ sửa adapter.
5. Tạo fake adapter cho test.
6. Chuẩn hóa lỗi và retry policy.
7. Không log raw HTML nếu chứa user data hoặc token.

## File cần sửa

- internal/services/chrome/live_metrics.go
- internal/services/chrome/keepalive_worker.go
- internal/services/chrome/login_manager.go
- internal/services/account_service.go

## File cần viết mới

- internal/providers/provider.go
- internal/providers/google/session_adapter.go
- internal/providers/google/flow_adapter.go
- internal/providers/google/gemini_adapter.go
- internal/providers/google/parsers/flow.go
- internal/providers/google/parsers/gemini.go
- internal/providers/google/errors.go
- internal/providers/fake/fake_adapter.go

## Gate hoàn thành

- Fake adapter chạy được toàn bộ job lifecycle.
- Provider-specific selector không xuất hiện trong scheduler.
- Provider error được map nhất quán.
- Snapshot thiếu dữ liệu được đánh dấu UNKNOWN, không tự biến thành 0.
- Một thay đổi parser không làm hỏng database/scheduler.

# Phase 4 — Job system cho một account

## Mục tiêu

Hoàn thiện request/job end-to-end với một account trước khi bật pool.

## Gemini request flow

~~~text
API request
  -> validate request
  -> create request id
  -> verify Gemini session
  -> acquire single-account lease
  -> execute/stream
  -> record usage
  -> release lease
  -> return response
~~~

## Flow job flow

~~~text
POST /v1/flow/jobs
  -> validate request
  -> create durable job
  -> acquire lease
  -> submit to Flow
  -> persist provider job id
  -> poll
  -> download/store output
  -> validate media artifact
  -> record usage
  -> release lease
~~~

## Job states

~~~text
QUEUED
STARTING
RUNNING
POLLING
SUCCEEDED
FAILED
CANCELLED
EXPIRED
RECOVERY_REQUIRED
~~~

## Việc cần làm

1. Idempotency key theo tenant.
2. Persist provider job ID ngay sau submit.
3. Heartbeat khi job đang chạy.
4. Job timeout.
5. Retry budget giới hạn.
6. Cancel semantics.
7. Restart recovery.
8. Không tạo job mới nếu request cũ đã hoàn tất.
9. Lưu output vào thư mục artifact riêng.
10. Dùng FFprobe kiểm tra:
    - file tồn tại;
    - container;
    - codec;
    - duration;
    - kích thước;
    - decode được.

## File cần viết mới

- internal/jobs/job_manager.go
- internal/jobs/job_state.go
- internal/jobs/idempotency.go
- internal/jobs/recovery.go
- internal/jobs/artifact_validator.go
- internal/jobs/job_manager_test.go

## Gate hoàn thành

- Kill gateway giữa submit và poll không làm mất provider job ID.
- Restart gateway tiếp tục poll job.
- Cùng idempotency key không tạo duplicate.
- Cancel không làm mất artifact đã hoàn tất.
- Artifact Flow được decode và kiểm tra thực tế.

## Stop rules

- Không mở pool nếu một job đơn lẻ còn có thể bị duplicate sau restart.
- Không báo SUCCEEDED chỉ vì provider trả status thành công nhưng artifact lỗi.

# Phase 5 — Account pool và scheduler

## Mục tiêu

Điều phối nhiều account/service bằng lease, health và quota snapshot.

## Nguyên tắc chọn account

Scheduler phải xét:

1. service/capability phù hợp;
2. session READY;
3. không DISABLED, EXPIRED, CHALLENGE_REQUIRED;
4. không trong cooldown;
5. concurrency còn trống;
6. quota/credit snapshot đủ và chưa quá stale;
7. health score;
8. fairness theo last_used_at;
9. giới hạn tenant/user.

Không sử dụng email hoặc random round-robin đơn giản làm chính sách duy nhất.

## Lease model

~~~text
acquire lease
  -> persist lease
  -> heartbeat while running
  -> release on success/failure
  -> expire automatically if process dies
~~~

Lease cần có:

~~~text
account_id
service
job_id
acquired_at
heartbeat_at
expires_at
released_at
~~~

## Cooldown và lỗi

| Tình huống | Hành động |
|---|---|
| Session expired | Đưa vào refresh queue |
| CAPTCHA/2FA | Chuyển CHALLENGE_REQUIRED, yêu cầu admin |
| Quota unknown | Giảm priority hoặc không chọn cho job đắt |
| Credit thấp | Chỉ nhận job nằm trong ngưỡng an toàn |
| HTTP 429 | Backoff/cooldown theo policy, không chuyển account tức thời để né giới hạn |
| Network error | Retry giới hạn, không đánh dấu account expired ngay |
| Nhiều lỗi liên tiếp | Circuit breaker/quarantine |
| Provider HTML thay đổi | PROVIDER_CHANGED, dừng adapter, không retry vô hạn |

## Việc cần làm

1. Implement PoolScheduler.
2. Implement LeaseManager.
3. Implement cooldown store.
4. Implement circuit breaker.
5. Thêm per-account concurrency.
6. Thêm per-tenant quota.
7. Thêm stale snapshot threshold.
8. Thêm account health score.
9. Thêm pool observability.
10. Test 2 account fake trước khi chạy nhiều account thật.

## File cần viết mới

- internal/scheduler/pool_scheduler.go
- internal/scheduler/lease_manager.go
- internal/scheduler/cooldown.go
- internal/scheduler/circuit_breaker.go
- internal/scheduler/selection_policy.go
- internal/scheduler/pool_scheduler_test.go

## Gate hoàn thành

- Không có double lease.
- Không vượt max_concurrency.
- Lease hết hạn sau process crash.
- Account lỗi bị quarantine.
- Scheduler phục hồi khi account trở lại READY.
- Không mất job khi scheduler restart.
- Test fake 2 account pass với load đồng thời.

# Phase 6 — Gateway API và tenant isolation

## Mục tiêu

Biến hệ thống thành gateway có API authentication riêng, không dùng local admin session làm API auth.

## API đề xuất

| Endpoint | Mục đích |
|---|---|
| GET /v1/models | Trả capability đã được gateway expose |
| POST /v1/gemini/generate | Gemini request hoặc stream |
| POST /v1/flow/jobs | Tạo Flow job bất đồng bộ |
| GET /v1/jobs/:id | Xem trạng thái job của tenant |
| POST /v1/jobs/:id/cancel | Hủy job |
| GET /v1/usage | Usage của tenant hiện tại |
| GET /healthz | Process health |
| GET /readyz | Database/provider readiness |

Admin endpoint phải nằm namespace riêng và không được expose cho API key người dùng.

## API key model

~~~text
gateway_api_keys
  id
  tenant_id
  key_prefix
  key_hash
  scopes
  rate_limit
  expires_at
  revoked_at
  created_at
~~~

Chỉ lưu hash/key prefix. Không lưu plaintext API key.

## Việc cần làm

1. Auth middleware.
2. Scope checking.
3. Tenant lookup.
4. Per-tenant rate limit.
5. Request size limit.
6. Request timeout.
7. Idempotency header.
8. SSE/polling cho job.
9. Error contract thống nhất.
10. Không trả account ID nội bộ hoặc provider credential.
11. Giữ bind 127.0.0.1 cho tới khi TLS/reverse proxy/auth hoàn chỉnh.

## File cần viết mới

- internal/api/http_server.go
- internal/api/auth.go
- internal/api/routes.go
- internal/api/tenant.go
- internal/api/errors.go
- internal/api/middleware.go
- internal/api/api_test.go

## Gate hoàn thành

- Không có API key thì bị từ chối.
- Tenant A không đọc được job/usage của tenant B.
- API key không gọi được admin methods.
- Không có cookie/token trong mọi response.
- Rate limit gateway độc lập với provider quota.
- Job API tiếp tục hoạt động sau gateway restart.

# Phase 7 — Security, privacy và vận hành

## Mục tiêu

Đặt security boundary đủ rõ trước khi đưa gateway ra mạng hoặc cho người dùng khác sử dụng.

## Credential security

- Tiếp tục mã hóa AES-GCM với key được bảo vệ bởi DPAPI trên Windows.
- Không ghi cookie/token/proxy vào log.
- Không đưa credential vào panic/error message.
- Không backup profile/cookie mặc định.
- Nếu export bắt buộc, tạo archive encrypted ngay từ đầu hoặc temp directory ACL riêng.
- Startup cleanup các temp backup/import còn sót.
- Không nhận Google password.

## CDP và browser security

- Dùng pipe hoặc xác minh PID/profile/nonce.
- Không bind CDP ra LAN.
- Không giữ debug endpoint sau khi hoàn thành.
- Kiểm tra profile lock trước khi mở.
- Kill process tree khi session/job bị hủy.
- Ghi nhận orphan Chrome process.

## Import/export security

- Giới hạn ZIP size.
- Giới hạn số entry.
- Giới hạn uncompressed size.
- Chặn path traversal.
- Chặn symlink/reparse point.
- Validate account/session trước khi copy profile.
- Cleanup khi import fail giữa chừng.

## Privacy và tenant

- Tách dữ liệu tenant.
- Usage log không chứa prompt nhạy cảm nếu không cần.
- Cho phép policy xóa job/artifact.
- Ghi rõ dữ liệu nào được chuyển tới Flow/Gemini.
- Không dùng account cá nhân của người khác nếu chưa có ủy quyền.

## Observability

Metrics tối thiểu:

~~~text
login_success_total
login_failure_total
session_refresh_success_total
session_refresh_failure_total
provider_request_total
provider_error_total
provider_rate_limited_total
quota_snapshot_stale_total
flow_credit_mismatch_total
job_started_total
job_succeeded_total
job_failed_total
job_recovered_total
lease_expired_total
orphan_browser_total
~~~

Log fields nên có:

~~~text
request_id
job_id
tenant_id
service
provider_account_internal_id
status
error_code
duration_ms
~~~

Không log:

~~~text
cookie
SNlM0e
API key
proxy password
Google password
raw authorization header
~~~

## Gate hoàn thành

- Secret scanning trên source/log/fixture không phát hiện secret.
- Không có CDP endpoint mở sau job.
- Backup/import crash test không để plaintext credential.
- Tất cả admin/user boundary đều có test.
- Có health/readiness khác nhau.
- Có cảnh báo khi quota stale, account challenge hoặc orphan process.

# Phase 8 — Kiểm thử, E2E và release readiness

## Mục tiêu

Chỉ công nhận hệ thống hoạt động khi có evidence từ code test, runtime, process và artifact.

## Unit test

Phải có test cho:

- cookie/domain/path policy;
- identity matching;
- session state machine;
- provider error mapping;
- quota snapshot confidence;
- lease acquire/release/expiry;
- scheduler selection;
- cooldown;
- circuit breaker;
- retry budget;
- idempotency;
- API-key hashing;
- tenant isolation;
- database migration;
- artifact validation.

## Integration test

Phải có fake hoặc test server cho:

- CDP target discovery;
- WebSocket CDP message;
- Chrome profile lock;
- proxy auth failure;
- process kill tree;
- provider session expiry;
- provider HTML/JSON change;
- gateway restart;
- job recovery;
- orphan cleanup.

## E2E opt-in

Không đưa Google credential thật vào CI. E2E thủ công/opt-in cần kiểm tra:

1. Login Google.
2. Verify identity.
3. Lấy Flow credits.
4. Lấy Gemini quota.
5. Refresh session.
6. Submit một job nhỏ.
7. Poll đến terminal state.
8. Validate output artifact bằng FFprobe.
9. Cancel một job đang chạy.
10. Logout/revoke.
11. Restart app.
12. Xác nhận không còn Chrome process hoặc plaintext temp file.

## Load test

Load test không được gửi tải lớn tới provider thật. Dùng fake adapter để kiểm tra:

- nhiều request đồng thời;
- fairness giữa account;
- lease timeout;
- scheduler restart;
- retry storm;
- tenant rate limit;
- backpressure;
- job queue growth.

## Release gate

| Gate | Điều kiện |
|---|---|
| Source | Không có secret, không có debug endpoint ngoài ý muốn |
| Database | Migration và rollback evidence đầy đủ |
| Session | Login/refresh/revoke/challenge đúng trạng thái |
| Provider | Flow/Gemini adapter contract pass |
| Scheduler | Không double lease, có cooldown và recovery |
| API | Auth, scope, tenant isolation pass |
| Runtime | Không orphan process, không rò credential |
| Artifact | Media decode được, duration/codec đúng |
| Build | go test, race, vet, frontend build, Wails build pass |
| E2E | Có evidence thật hoặc trạng thái NOT RUN/BLOCKED rõ ràng |

Không đánh dấu PASS chỉ vì UI hiển thị “Hoàn tất”, test unit pass hoặc job có status thành công.

## Kế hoạch thực hiện ưu tiên

### P0 — Bắt buộc trước pool

1. Baseline fixture và _progress.md.
2. Identity verification.
3. Session state machine.
4. Secure CDP.
5. Quota/credit snapshot có source/time/confidence.
6. Sửa go vet.
7. Không còn secret trong log/fixture.

### P1 — Bắt buộc trước gateway API

1. Database migration versioning.
2. Provider adapters.
3. Single-account job system.
4. Job recovery/idempotency.
5. Artifact validation.
6. API key/auth/tenant isolation.

### P2 — Bắt buộc trước account pool thật

1. Account lease.
2. Scheduler.
3. Cooldown/circuit breaker.
4. Per-account concurrency.
5. Fake two-account load test.
6. Reconciliation profile/session.

### P3 — Bắt buộc trước public exposure

1. Security review.
2. E2E opt-in.
3. Crash/restart test.
4. Secret/backup/import review.
5. TLS/reverse proxy.
6. Privacy/terms review.
7. Runbook vận hành.

## Stop rules chung

Dừng mở rộng phase tiếp theo nếu xảy ra một trong các điều kiện:

- session được đánh dấu READY nhưng identity chưa được xác minh;
- cookie/token xuất hiện trong log, fixture, response hoặc backup plaintext;
- CDP endpoint còn mở sau job;
- job có thể bị duplicate sau restart;
- scheduler cấp cùng một lease cho hai job;
- account pool tự chuyển account chỉ để né 429;
- quota/credit không phân biệt được UNKNOWN và ZERO;
- provider thay đổi nhưng hệ thống vẫn âm thầm báo thành công;
- output artifact không decode được;
- migration làm mất hoặc ghi đè dữ liệu account;
- có orphan Chrome process không được giải thích;
- test/build quan trọng fail nhưng vẫn tiếp tục release.

## Checklist triển khai đầu tiên

~~~text
[ ] Tạo baseline fixture đã redacted
[ ] Tạo _progress.md
[ ] Chuẩn hóa state/status
[ ] Xác minh identity thật
[ ] Tách Flow session và Gemini session
[ ] Hardening CDP
[ ] Thêm quota snapshot source/time/confidence
[ ] Sửa go vet/toolchain
[ ] Viết provider adapter contract
[ ] Viết fake provider adapter
[ ] Viết job manager một account
[ ] Viết idempotency và recovery
[ ] Viết database migration
[ ] Viết lease manager
[ ] Viết scheduler với fake accounts
[ ] Viết API auth và tenant isolation
[ ] Viết artifact validator
[ ] Chạy crash/restart test
[ ] Chạy E2E opt-in
[ ] Chạy release gates
~~~

## Tiêu chí kết thúc toàn bộ Phase 0–Phase 8

Hệ thống chỉ được xem là đạt mục tiêu khi:

1. Một account có thể login, verify, refresh và revoke ổn định.
2. Flow và Gemini là hai provider capability độc lập.
3. Cookie/session không bị expose ra public API.
4. Quota/credit có snapshot, nguồn, thời gian và confidence.
5. Job Flow không mất trạng thái khi gateway restart.
6. Gemini request có timeout, retry và usage record.
7. Pool có lease, cooldown, health, fairness và recovery.
8. Account bị challenge hoặc expired không bị dùng tiếp.
9. Tenant không thể đọc dữ liệu tenant khác.
10. Backup/import không để credential plaintext.
11. Không có orphan browser process sau test.
12. Artifact media được xác minh bằng công cụ decode thực tế.
13. Build/test/review gates đều có evidence.
14. Các giới hạn provider và trạng thái NOT RUN/BLOCKED/PARTIAL được ghi rõ.

## Trạng thái báo cáo chuẩn

Sử dụng các trạng thái sau trong _progress.md và báo cáo:

| Trạng thái | Ý nghĩa |
|---|---|
| NOT STARTED | Chưa thực hiện |
| IN PROGRESS | Đang thực hiện, chưa đủ gate |
| PASS | Có evidence đầy đủ và gate đạt |
| PARTIAL | Một phần đạt, còn thiếu gate |
| BLOCKED | Không thể tiếp tục do provider/runtime/authority |
| NOT RUN | Chưa chạy, không được suy diễn là pass |

Tài liệu này là kế hoạch triển khai. Việc sửa source, migration database, xóa profile, mở cổng mạng, commit, push, deploy hoặc release cần được thực hiện ở các bước riêng với evidence tương ứng.
