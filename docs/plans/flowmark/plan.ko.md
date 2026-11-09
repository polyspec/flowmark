<!-- doc-id: plans-design-plan -->
<!-- source-sha256: e155a4ff680241ff1be8a47469514f3c6b53e685ccf01d0550820813beab2364 -->
# flowmark 설계 계획

Revision 2 — 3개 lane의 적대 감사 결과(engine core, orm 통합, 운영)와 multi-platform 지시를 아래 판정에 접는다. 감사의 blocker는 전부 판정이 되었고, 부수지 못한 주장은 문서 끝에 남긴다.

Revision 3 — [markdown database 계획](../markdown-database/plan.ko.md)이 철회한 판정을, 그것을 철회하게 한 사실과 함께 정정한다:

- **Seam.** 이 계획은 markdown executor를 orm 안에 두었다. orm은 query layer다: client는 자기 process 안에서 plan하고 모든 database에 driver로 접속한다(orm `docs/protocol.md`, `docs/config.md`). 그래서 orm 안의 executor는 client마다 하나씩 존재하게 된다 — 아래의 "Go-only" 단서가 그 증상이었다. code도 그 판정을 따른 적이 없다: orm은 dbspec setting 두 개를 받았고 flowmark는 orm에서 아무것도 import하지 않았다. flowmark가 엔진이고 orm은 DSN으로 IR executor로서 접속한다.
- **Schema stamp와 단독 fragment.** 모든 문서의 stamp는 schema를 행마다 복사한다. corpus root의 `dbspec/` catalog가 유일한 schema다.
- **task-ID 문법.** key는 불투명하다. 계층은 `parent` column이고 대체는 link이므로 ID 문법, 그 정렬, renumber codemod는 사라진다.
- **`identity` 거절, advisory `audit`, column limit으로서의 WIP gate, operation 단위만의 lock, corpus revision**은 계획이 적은 대로 대체한다.
- **T8.5** — 문서 안 Mermaid 블록을 전이 authority로 쓰는 것 — 은 이 계획의 판정이 아니었으며 함께 철회한다.
- **읽을 수 있는 문서.** corpus 문서는 일반 markdown 문서처럼 읽히는 `*.flowmark.md` file이다: doc-id header, 행 file의 frontmatter, provenance header, twin, `dbspec/flowmark.json`은 철회된다. `files` 행의 key는 file 이름이고 foreign key는 link다([markdown database 계획](../markdown-database/plan.ko.md), "읽을 수 있는 문서").

아래 절은 정정된 판정을 담고, spec이 그 전부를 담는다.

## 배경

사용자는 markdown으로 engineering spec을 관리하며 checklist(상태 5종 `[ ]` `[~]` `[o]` `[!]` `[-]`, dotted 계층 ID, 선행/후속 의존), changelog, commit 규칙으로 개발을 운영한다. 문제: 문서 기반 운영에는 **멱등하고 DB 수준의 강제력이 없다** — 형식 위반, dangling foreign key, 불법 status 전환이 아무도 막지 못한다.

**flowmark의 정체성 (사용자 확정)**: **markdown 문법을 가진 텍스트 DB**. 파일 자체가 database이며 외부 DB 없이 standalone으로 완전 동작한다. **flowmark가 엔진**이고(Revision 3 — orm은 IR executor로서 접속하는 query layer) **통합·응용 layer**다 — database, GitHub, Jira 등 tracker와의 양방향 동기화, 그리고 그 위에서 local 칸반 보드와 issue tracker(GUI는 또 하나의 thin consumer): GitHub과 Jira에도 보드는 있지만 여기서는 data를 로컬에서, git 안에서, 오프라인으로 관리한다. 핵심 가치: 저장 형식이 사람이 읽는 문서이고, git이 audit·branching·rollback을 제공한다.

**사용자 결정사항**:

1. polyspec/orm은 markdown에 mysql/pgsql/sqlite 옆의 **4번째 database**로 접속한다 — flowmark가 서비스하는 IR executor를 고르는 DSN `markdown://`을 통해서(Revision 3). face는 자체 write logic이 없는 thin adapter다.
2. Schema 방향: **dbspec이 root grammar이자 유일한 authority** — corpus root의 `dbspec/` catalog가 모든 table과 그 저장을 선언하며 문서는 schema 사본을 갖지 않는다 (상세 §4, Revision 3).
3. 문서는 **EN/KO 쌍을 day 1부터** (orm house style, kit 호환).
4. **polyspec/kit을 지금 vendor** — 5번째 상태 `[-]`는 **polyspec/kit 저장소에 upstream** 한다 (orm의 `scripts/kit`은 hash-lock된 vendor copy라 orm에서 직접 고쳐선 안 되고 `make kit-sync KIT_TAG=<tag>`로만 반영된다). 사용자가 kit에 대한 권한을 위임했다.
5. 이번 session의 deliverable = **설계 문서 + repository bootstrap** (구현은 checklist의 후속 phase로 추적). 감사의 범위 지적에 따라 spec 문서 자체는 다음 session(T1b)으로 분리하고, 이번 session은 T1a를 닫는다.
6. Sync projection 파일: 실제 markdown 파일이 disk에 존재. `readonly`(기본) / `writable` 옵션 (상세 §7).
7. Repository는 **github.com/polyspec/flowmark** — polyspec organization 안의 독립 repository.
8. **Multi-platform은 hard requirement. 편법 금지.** 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다 (kit 자신의 규칙을 여기에 적용): OS file lock 없음, POSIX 전용 durability 가정 없음 — lock은 kit의 `holder-lock` 선례(lock file + holder 신원 + stale 판정)를 따르며, platform마다 필요한 primitive는 spec 40에 이름을 밝힌다(lock 대기를 이끄는 변경 알림, holder를 판정하는 process start와 machine 식별자, Windows에서 이름을 지속시키는 write-through move). crash 안전성은 journal replay로 얻으며, spec 40이 platform별로 정하듯 각 단계의 file과 이름을 다음 단계 전에 지속시킨다.
9. **GUI는 자기 repo의 soksak plugin으로 출시되고, flowmark는 soksak의 sidecar로 서비스한다.** 하나의 Go binary가 세 개의 thin face을 갖는다 — CLI, MCP server, soksak sidecar(soksak core 명세의 protocol v1: 표준 입출력 위의 JSON lines, surface·root routing, host 소유 lifecycle). 저장소는 단일 언어 Go에 `package.json`과 `sidecar.json` manifest를 얹은 형태이고, 언어별 package는 요구사항이 이름을 붙일 때 만든다.

## 검증된 근거 (polyspec checkout에서 직접 확인)

- `orm/docs/protocol.md:12,96` — IR kind는 bounded: `one|all|count|group_count|sum|avg|paginate|insert|update|delete|restore`. request는 SQL text를 담지 않는다 → **IR request를 소비하는 native markdown executor는 bounded 문제**. 감사가 표현력을 확인했다: 전 kind와 Order/Pred/Func 표면이 in-memory corpus 위에서 계산 가능하며, 열린 문제는 명세였지 표현력이 아니었다.
- `orm/docs/dialects.md:184` — 3개 database가 collation을 binary codepoint order로 통일한다. 이 규칙은 **text 비교**의 규칙이며 엔진이 따른다 (§2의 ordering 판정 참조).
- `kit/scripts/kit/documents-stamp.mjs`, `check-documents.mjs` — `source-sha256`는 **EN→KO 번역 sync marker**(Korean twin이 English 파일 raw bytes의 sha256를 보유). 데이터 drift 감지는 `.flowmark/` sidecar state로 설계하며, flowmark는 같은 marker에 **raw 파일 bytes hashing**을 쓴다 (kit 호환; 정규화는 hash가 아니라 검사에서 한다).
- `kit`은 독립 저장소(`polyspec/kit`)이며, consumer는 `make kit-sync KIT_TAG=<tag>`가 쓰고 `make kit-check`가 검증하는 `scripts/kit` + `tests/kit`의 hash-lock vendor copy를 가진다. vendored file은 kit에서만 바꾼다.
- `kit/scripts/kit/checklist.mjs` — 상태는 tracker별 data다. `[-]` 불가 상태는 이제 두 수준에서 읽힌다 (kit K13으로 upstream, branch `feat/fifth-state-K13` merge 대기 중).
- kit 문서 규칙: 모든 English 문서에 Korean twin 필요(doc-id 일치, explicit anchor 일치, fenced block 일치, twin이 source-sha256 보유), plans/와 CHANGELOG도 twin 검사 대상.

## 핵심 설계 (spec 문서가 확정할 내용)

### 1. Architecture와 seam

```
application code ── orm client (Go/PHP/Python/Rust/TS) ── IR executor ── DSN markdown:// ──┐
                                                                                          ▼
flowmark = the markdown database engine (one Go binary)
   ├─ codec: strict parser + canonical renderer of the storages (two-clause idempotency, §2)
   ├─ validator: catalog constraints (type/PK/UNIQUE/FK/CHECK)
   ├─ IR executor over in-memory indexes derived from the parsed corpus
   ├─ write path: lock → conflict check → validate → stage → prepare record → renames → commit
   ├─ .flowmark/: lock, tx journal, server token
   ├─ faces: server · CLI · MCP · soksak sidecar
   └─ sync engine: md→db export / db→md materialize (§7)
```

**Seam 판정 (Revision 3)**: orm은 query layer다. client는 자기 process 안에서 plan하고 DSN scheme이 고른 driver로 각 database에 접속한다. scheme `markdown://`은 **IR executor**를 고른다: client는 IR 요청을 검증하고 plan하지 않으며 flowmark server로 보내고, server는 행이나 orm 오류 하나로 답한다 — `getQuery()`, statement event, `subscribe()`, `force_index`는 `CAPABILITY_UNSUPPORTED`다(지원 불가 능력에 대한 orm의 선례). 엔진은 flowmark에 한 번 존재하고, 각 orm client는 얇은 executor client를 가지며 Go client가 먼저다. flowmark는 orm의 `engine/dbspec`와 `engine/ir` package를 library로 쓴다. Revision 2는 executor를 orm에 두었고, 그러면 다섯 client 각각에 엔진이 들어갔을 것이다.

- SQLite mirror는 부정한다: mirror↔corpus 동등성이 영구적인 bug class가 된다. mirror를 인정하더라도 conformance 조건은 (a) full-rebuild equivalence (b) write path 부재다.
- IR boundedness 주장은 request layer만 대상으로 한다. **API 동등성은 주장하지 않는다** — capability table(§6)이 계약이며, orm의 `CAPABILITY_UNSUPPORTED`/`OPERATOR_NOT_ALLOWED` 전통을 따른다.

**예제 판정 (T77)**: flowmark는 엔진과 엔진에 필요한 의존만 갖는다: 자기 Go module과, 그 `engine/dbspec`과 `engine/ir`을 쓰는 orm Go module이다. 예제 — tracker와 board corpus, 그 Go test, 그리고 hyper, crudui, orm, template, php-serialize, ordered-json을 고정하고 PHP와 Composer가 필요한 board web application — 는 flowmark의 tag된 Go module을 소비하는 저장소 flowmark-examples에 둔다.

### 2. Canonical format (`10-canonical-format.md`)

- **멱등성은 dbspec의 2절 명제다** — 단일 byte-identity 주장이 아니라: canonical 입력 `s`에 대해 `emit(parse(s)) == s`, 임의의 valid 입력 `d`에 대해 `emit(parse(emit(d))) == emit(d)`. renderer는 **구조 skeleton** — `files` 행의 field list, heading line, 행들의 item/table line — 만 소유하고(Revision 3), 보존 구간의 payload bytes(prose, fenced 내용)는 결코 다시 쓰지 않는다. 비-canonical 입력은 `format.noncanonical` diagnostic과 별도의 일회성 `flowmark canon` migration transaction으로 처리하며, 논리적 write의 부수 효과로 정규화하지 않는다.
- body와 모든 `markdown` column 값은 fenced block 밖의 heading level로 경계가 정해지는 행 scope의 섹션이다. 없는 섹션은 NULL, 비어 있는 섹션은 빈 text다 (Revision 3).
- **Ordering**: IR `order`는 column의 type으로, text는 codepoint로 정렬한다. 없는 읽기는 저장 순서 — 형제를 order column이나 key 순서로 둔 tree의 전위 순회 — 를 돌려준다. `group_count`는 NULL을 같은 group으로 처리한다 (SQL 동등성). key는 불투명하다: task-ID 문법, 그 segment별 정렬, suffix 규칙, `flowmark next-id`는 철회한다(Revision 3). `identity`와 `key_prefix`가 key를 할당한다.
- **List 행**은 상태 glyph, key, 제목, 오른쪽부터 parse하는 끝의 `column:value` field를 갖는다. `markdown` column은 field나 cell이 아니며, 한 줄 자리에 줄바꿈을 가진 값은 거절한다.
- **읽을 수 있는 값** (Revision 3): corpus 문서에는 frontmatter도 comment도 없다. 일반 markdown 문서처럼 읽힌다. `files` 행의 scalar는 H1 뒤의 field list이며, dbspec literal이 아니라 읽을 수 있는 형식(날짜, uuid, `bytes`는 padding 있는 base64)으로 쓴다. 모든 관계는 link다.
- corpus 문서에는 twin이 없다 (Revision 3): 행 file은 doc-id도 `source-sha256`도 갖지 않는다. EN/KO 쌍과 `source-sha256`(English 파일 raw bytes의 sha256, kit 호환)은 이 repository 자신의 문서에 속한다.

### 3. Document-table model (`20-document-model.md`)

- 각 table은 저장 하나를 선언한다(Revision 3): `store files`(table 폴더에 행마다 file 하나), `store document list|table`(모든 행이 `<table>.flowmark.md`에), `store block <fk> list|table`(부모 행 안의 행). aggregate냐 split이냐는 저장 선언이며 논리 관계는 같다.
- `files` 행은 file `<table>/<key>.flowmark.md`이며, H1이 제목이고 scalar column이 그 뒤의 field list다 (Revision 3). key는 file 이름이고, 계층은 `parent` column이며 list에서는 들여쓰기로 쓴다.

### 4. Schema (`30-schema.md`) — catalog가 schema다

- **dbspec이 root grammar이자 유일한 authority.** corpus root의 `dbspec/*.dbs` set이 catalog다(Revision 3). 문서는 schema 사본도 stamp도 갖지 않고, 내장 fragment를 가진 단독 문서는 철회한다: schema는 law이고 data는 row이며, 문서마다 law의 사본을 두면 사본의 이탈이 그 자체로 bug class가 된다. orm의 engine, IR, conformance는 dbspec manifest hash에 묶여 있고, dbspec의 parser와 type system은 library로 재사용한다.
- **Markdown 고유 schema 표면은 manifest-level setting이지 render되는 type이 아니다** (introspection 정합성, `updated`/`soft_delete` 선례): `store`, `key_prefix`, `title`, `body`, `order`, `checkbox`, `markdown`은 SQL backend가 무시하는 setting이며, 실제 database는 이들을 평범한 column으로 introspect한다. 상태기계의 setting은 철회하고, `key_prefix`와 `order`는 판정: 엔진은 흔한 database가 가진 것만 가진다(T62)가 정한다.
- **`identity`는 허용한다** — markdown table의 `i64` primary key에만, lock 아래에서 가장 큰 key와 표의 AUTOINCREMENT counter 가운데 큰 것보다 하나 큰 key로 할당한다(Revision 3, counter는 바뀐 행의 refresh(T42); 다른 key type에는 더할 최댓값이 없다). git 되감기에서의 key 재사용은 받아들인다. 되감기가 counter도 되감기 때문이다 — 할당 기록은 git history다.

### 5. Ops와 transaction (`40-ops.md`)

- **Commit protocol** (순서가 규범적): rename n마다 source로 post-image의 사본 `.flowmark/tx/<id>.<n>.<hash>`를 쓰고 sync하여 같은 두 대상이 두 file로 남고 어느 대상도 stage와 file을 나누지 않게 한다; commit이 받는 generation, 모든 rename과 pre/post hash(없는 file은 빈 hash), 기록된 두 상태의 byte를 적은 **prepare record** `<id>.prepare`를 쓰고 그 folder와 함께 sync한다; runtime의 atomic rename으로 rename하며(delete는 journal로의 rename) 대상의 folder를 한 번씩 sync한다; generation을 쓴다; stage 정리(T58, 판정: commit은 원자성이 요구하는 만큼만 disk를 기다린다). **복구는 corpus를 열 때마다 실행한다**: prepare record가 있으면 완료, 없으면 stage 폐기, 대상의 현재 hash가 기록된 어느 쪽과도 다르면 기록된 두 상태를 담은 stage file을 밝히며 거부하고 보고한다(crash 사이에 변경이 들어왔다는 뜻 — 결코 조용히 덮어쓰지 않는다). 정전 이후의 durability는 각 단계의 file과 이름이 다음 단계 전에 disk에 있기를 요구한다: Linux와 macOS에서는 folder sync, Windows에서는 write-through move다(spec 40).
- **하나의 portable lock** (multi-platform 판정): 모든 작업 — 읽기와 쓰기, 어느 face에서든 — 은 holder 신원·stale 판정을 갖는 같은 lock file `.flowmark/lock`을 획득한다(kit `holder-lock` 선례). 잡힌 lock은 OS 변경 알림이 이끄는 대기로 holder를 보고하는 deadline까지 기다리며, stale lock은 `flowmark unlock`만 지운다. lock은 작업 하나 동안, 또는 server transaction 하나의 `begin`부터 `commit`이나 `rollback`까지 잡는다(Revision 3). 닫힌 연결이나 유휴 transaction은 rollback하고 lock을 푼다. commit 시 충돌(읽은 뒤 대상이 바뀜)은 file과 line과 함께 실패한다 — face는 다시 읽고 한 번 재시도하며 orm client는 `WRITE_CONFLICT`를 받는다 — silent merge 없음.
- **Server**(Revision 3)는 loopback에 bind하고 유지되는 token을 갖는 상주 face다. DSN은 `markdown://127.0.0.1:<port>/<corpus>?token=<token>`이다.
- **검증은 commit 시점에 충돌 검사 뒤 lock 아래에서 corpus를 새로 읽어 수행한다** — BEGIN snapshot도 예측된 touch file 집합도 아니다.
- corpus revision과 `CORPUS_CHANGED`는 철회한다(Revision 3): `paginate`는 요청 하나이고, 호출을 넘어 읽는 face는 없다.
- **비용 모델**: 40-ops가 per-command p95 latency 목표를 선언한다(corpus envelope 기준 수백 ms 단위); operation이 변하지 않은 file을 읽지 않는 방법 — file 알림으로 최신을 유지하는 오래 사는 corpus이며 commit 대상은 언제나 lock 아래에서 다시 읽는다 — 은 `T12.5-2`가 맡는다. content hash를 key로 하는 cache도 모든 file을 읽어야 하기 때문이다; commit은 transaction이 바꿀 수 있는 모든 제약을 lock 아래 다시 읽은 corpus에서 검사하며, 전체 `flowmark validate`는 on demand와 catalog 변경 시에 실행한다.
- Walk 규칙: `dbspec/`, `.flowmark/`, 숨김 항목 밖의 `*.flowmark.md` 파일만 읽는다; symlink된 문서나 `files` 폴더는 `corpus.walk`이고, 그 밖의 symlink는 database 밖이다; hard link는 규칙이 아니다(document model).

### 6. Constraints와 capability table (`50-constraints.md`, `60-dialect.md`)

- Strict parse, type/PK/UNIQUE/FK(exact byte match; 대소문자 무시 시 key 유일), CHECK. soft-delete된 row는 unique key를 계속 차지하고 FK를 계속 만족한다(SQL 동등성 — soft delete가 가짜 판정을 일으키지 않게 명시). `aes`, `aes_version`, `blind_index`는 corpus에 선언한다: client가 SQL과 같이 암호화하고, 엔진은 주입으로 key를 받는다(spec 30 Key).
- 엔진은 **상태기계를 갖지 않는다**(T62, 판정: 엔진은 흔한 database가 가진 것만 가진다): 전이, terminal, 초기 상태, 필수 column, limit을 강제하지 않고, 이력 행을 쓰지 않으며, 이력 빈틈이나 사슬을 보고하지 않는다. 상태의 전이와 그 이력은 polyspec fsm의 것이다. 이로써 상태기계가 엔진이 강제하는 dbspec-level manifest setting이라는 판정(Revision 2와 3)을 철회한다. 엔진은 WIP 규칙을 강제하지 않으며 사용자의 규칙은 작업 규칙으로 남는다.
- **사람의 write는 DB의 writer 독점 전제를 무효화한다; 각 기능은 그에 대한 판정을 갖는다**: FK action은 **restrict만 허용**(`cascade`/`set_null`은 catalog를 읽을 때 거부 — 엔진을 우회하는 writer에게 구현 불가능하므로); `audit`와 `immutable`은 corpus에서 schema 오류였고 상태 이력이 대신했다(Revision 3). 문장 경로는 둘을 database처럼 받으며(T31: `immutable` 표의 행에 대한 엔진 쓰기는 거부하고, audit되는 쓰기는 database의 audit trigger가 쓰는 행을 쓴다), 상태 이력은 상태기계와 함께 철회한다(T62); 모든 non-null column은 쓰며 엔진이 insert에서 기본값을 쓴다.
- **Capability table**(`60-dialect.md`)은 IR executor wire 아래의 통합 계약이다: `update|share` lock과 `*_nowait`는 문서화된 no-op; `isolation`은 모든 level을 받아들이고(transaction이 corpus lock을 쥔다); `timeoutMs`는 `CAPABILITY_UNSUPPORTED`, `fulltext`는 `OPERATOR_NOT_ALLOWED`; `on_duplicate` insert는 충돌 key를 정확히 하나 가진다(orm의 규칙); 바뀐 대상은 `WRITE_CONFLICT`로 실패; `getQuery`, statement event, `subscribe`, `force_index`는 `CAPABILITY_UNSUPPORTED`; savepoint는 중첩 stage.
- markdown plan lifecycle(orm의 dbplan/apply/recover 장치)은 **v1 범위 밖**: v1은 corpus 생성과 additive column 추가를 지원하고, markdown corpus의 dbplan chain은 거부한다(80-migration이 범위와 이유를 서술).

### 7. Sync와 adapter (옵션, `70-sync.md`)

- sync engine은 **하나의 계약을 가진 adapter architecture**다; database가 첫 adapter이고 GitHub(issues/projects)과 Jira가 따른다: table별 truth ownership, 소유 system을 밝히는 문서 밖에 둔 sync provenance(그 위치와 형식은 open question 5(70-sync)가 정한다), git을 인식하는 drift 처리, merge base로 기록되는 materialized-from revision, 명시적 일회성 command(live daemon 없음). 외부 tracker의 항목은 corpus row로 materialize된다 — checklist app의 item model이 곧 local issue model이므로 local board와 remote tracker가 같은 row의 두 projection이다.
- md→db: derived export (재구축 가능). db→md(그리고 모든 adapter→md): materialize. validator는 adapter-owned 문서의 편집을 거부한다(readonly 기본); `writable`은 검증 후 re-ingest한다.
- **readonly drift는 git을 인식한다**: working tree가 HEAD와 다르면 HEAD로 복구하고 보고; tree가 깨끗한데 HEAD가 마지막 ingested version과 다르면 수용하고 re-baseline한다. adapter-owned path의 merge conflict는 양쪽을 버리고 re-materialize로 해결한다 — 그곳의 손 merge는 diagnostic이다.
- export는 catalog에서 orm의 SQL dialect renderer로 렌더한다(Revision 3). changelog projection은 이력 행 — 파일 안의 data — 에서 유도하며 gitignore된 journal에서 절대 유도하지 않는다(fresh clone이 모든 projection을 재구축할 수 있어야 한다).

### 8. Migration (`80-migration.md`)

- v1 범위: catalog에서 `flowmark install`로 corpus 생성, additive column 추가. 저장 이동은 transaction 하나다. renumber codemod는 ID 문법과 함께 철회한다(Revision 3). additive-first; rename은 명시적 step; dbplan chain은 `CAPABILITY_UNSUPPORTED`(§6 참조).

### 9. 응용 schema (`100-checklist.md`)

- `flowmark.dbs`(Revision 3, T62): `items`(parent, check가 제약하는 다섯 값의 상태, 제목, body, 담당자, date/datetime 쌍인 시작과 마감, priority, position), `people`, `item_links`(blocks, relates, duplicates, supersedes), `views`. 전이와 그 이력은 polyspec fsm의 것이므로 5상태 기계와 `item_history`는 상태기계와 함께 철회한다(T62).
- **View는 데이터이자 thin consumer다**: 칸반, 목록, 간트 view는 `views`의 행이다. 보드는 엔진으로 읽고 같은 operation으로 쓴다 — 불법 drag는 불법 CLI 쓰기와 똑같이 실패한다. view 전용 write path는 없고, WIP limit을 지어내지 않는다.
- 이 repository는 corpus가 아니다(Revision 3): 그 checklist는 손으로 관리하는 kit 문서로 남으므로 cutover는 철회한다. `make validate`와 prose ID checker는 옛 엔진과 함께 제거되며, `make refs-check`는 계속 kit이 추적하는 checklist를 검사한다.

## Session 계획 (이번 session은 T1a, 다음 session은 T1b)

**T1a — 감사 반영 설계 + bootstrap** (이번 session)

1. flowmark `git init`, `.gitignore`(`.flowmark/`), `README.md`+`README.ko.md`, `AGENTS.md`+`AGENTS.ko.md`(운영 규칙: checklist discipline, conventional commits, [o]↔changelog↔commit 단위, twin workflow — 한국어에서 발견한 수정은 다음 stamp 전에 English 파일로 이식, 문서 commit에 대한 interactive history 편집 금지), `CHANGELOG.md`+`CHANGELOG.ko.md`(`## Unreleased`). commit: `chore: Bootstrap flowmark repository`.
2. **kit `[-]` upstream** — branch `feat/fifth-state-K13`에서 완료(worktree `kit-fifth-state-K13`, red vector → FIVE_STATES + 원인 규칙 → green, K13 row + AGENTS 쌍 + stamp된 KO checklist). kit의 병렬 작업이 안정화되면 `main`에 merge하고 worktree와 branch를 즉시 제거한다. orm은 사용자가 kit release에 tag를 붙일 때 `make kit-sync`로 채택한다.
3. flowmark에 **kit 디렉터리 전체 vendor**(`scripts/kit` + `tests/kit` + `kit.json` + `.kit/kit.lock.json`, kit-sync 계약 준수, lock은 kit commit을 기록)하고 `config/checklist.json`(list format, `T<n>(.<n>)*(-<n>)?` ID pattern, 상태 5종, active `[~]`)과 `config/documents.json`을 작성한다. commit: `chore: Vendor polyspec/kit tooling`.
4. `docs/plans/flowmark/checklist.md`+`.ko.md` — 사용자 형식(doc-id, 선행/후속, 원인/완료 기준/Evidence)으로 T1a/T1b/T2–T9 항목을 담고 전 과정에서 갱신한다.
5. 이 설계 계획(revision 2)과 Korean twin — 감사를 접은 상태로 commit.

**T1b — spec 문서** (다음 session; EN/KO 쌍, 문서당 commit 1개, twin stamp): `00-overview`, `10-canonical-format`(§2), `20-document-model`(§3), `30-schema`(§4), `40-ops`(§5), `50-constraints`(§6), 그리고 skeleton `60-dialect`(capability table을 전체 내용으로), `70-sync`, `80-migration`, `90-conformance`, 전체 내용 `100-checklist`, `110-open-questions`. 문서별 완료 기준: 모든 판정이 근거를 동반한 명제로 나타난다 (prose 분량이 아니라 결정 수).

**판정: orm은 SQL로 corpus에 닿는다.** orm의 markdown 연결은 server가 문장에 답하는 SQLite 연결이므로([SQL statements](../../spec/40-ops.md#sql-statements)) orm의 recipe는 다른 database처럼 flowmark에서 실행되고 runtime은 driver 하나만 이식한다. 이 판정은 요청에 SQL text가 없다는 이전 판정을 대체한다. 이전 판정은 runtime마다 두 번째 executor client를 이식하게 했고 대부분의 recipe를 빠뜨렸다. IR은 엔진 자신의 face와 문장의 차이를 쓰는 writer로 남는다. 성능은 database 수준이 아니어도 되지만 동작은 database 수준이어야 한다.

**판정: 연산은 건드린 만큼 비용이 든다.** 문장과 엔진 face의 모든 연산은 corpus 크기가 아니라 자신이 바꾸는 파일과 건드리는 행만큼
비용이 들고, 읽기는 WAL mode의 SQLite처럼 writer 옆에서 실행된다. T31의 코드는 연산이 지나는 모든 계층에서 이것에 어긋나고, 어느
것도 측정된 적이 없다:

- lock 아래의 corpus 읽기(`writepath.Corpus.read`)는 `Begin`에서, 그리고 commit에서 다시 `validate.Read`(모든 파일에 대한 두 번의
  pass와 모든 행에 대한 availability 규칙)와 `irexec.Open`, `index`(모든 행)를 실행한다;
- 모든 쓰기 요청은 모든 행을 깊이 복사하고(`irexec.Corpus.clone`), commit은 모든 파일을 비교한다(`Corpus.Differs`);
- SQL 쓰기는 문장 전후에 모든 표의 모든 행을 읽고(`execute`, `allRows`, `diff`), `placeBack`은 순서 있는 표 전체를,
  `allocate`는 prefix의 모든 key를 읽으며, 바뀐 corpus는 새 database에 통째로 복사된다;
- 읽기는 lock 기록을 쓰고, 행이 이어지는 읽기는 닫힐 때까지 corpus lock을 쥐며, 두 번째 읽기는 첫 읽기의 cursor를 기다린다.
  database는 `database/sql` 뒤의 `:memory:`이고, `database/sql`은 그 연결을 빈 database로 바꿀 수 있다.

각 계층은 이 순서의 행이며, 각 행의 Red는 T36 비용 counter의 수치다:

1. **비용 counter(T36).** 엔진은 자신이 읽고 parsing하고 비교하는 파일, index하고 복사하는 행, 문장이 database에서 읽고 쓰는 행을
   센다. 시험은 한 행 읽기, 크기가 고정된 형제 묶음을 가진 순서 있는 표와 `key_prefix` 표에서의 한 행 insert, update, delete, 그리고
   알림으로 기다리는 파일 하나의 외부 편집을 1,000행과 100,000행에서 실행한다. 각 수치는 두 크기에서 같고, `testing.AllocsPerRun`은
   명시된 상수 이상 다르지 않다. 이 시험들은 아래 행들의 Red이며 행마다 green이 된다. benchmark는 시간을 보고하고 아무것도 막지
   않는다.
2. **commit 세대(T37).** journal은 선언된 위치에 세대를 알린다: counter와, 명시된 수만큼의 이전 세대까지 각 commit의 경로다.
   commit은 자신의 commit record에서, rename 뒤 stage를 수거하기 전에, 그 record의 fsync와 함께 세대를 올리고, commit을 마치는
   recovery도 그렇게 한다. corpus는 자신을 읽은 세대를 지닌다. 외부 편집은 commit을 거치지 않으므로 세대가 없고, tree의 알림으로
   알려진다.
3. **증분 corpus 읽기(T38).** 읽기는 지난 읽기 이후 tree의 알림이 이름 댄 경로와, 자신의 세대 이후 commit들의 경로를 받으며, 후자는
   먼저 tree에서 잊는다. 명시된 수보다 오래된 세대와 overflow는 tree를 다시 읽는다. `validate.Live`는 그 folder와 문서만 다시 읽고,
   한 읽기를 다른 읽기에 기대게 하는 것을 그것들에 대해 갱신하는 index로 유지한다: 각 key의 link target과 그 key를 물은 link가 있는
   문서(target이 바뀌면 정확히 그 문서만 다시 parse한다), 그리고 표를 쓸 수 없게 하는 중복 key와 읽을 수 없는 entry다.
   `irexec.FromSnapshot`은 바뀌지 않은 문서를 모두 이전 corpus와 공유한다. 증분 읽기는 처음부터 읽은 결과와 같고, 이는 표 형식이며
   fuzz된다. 읽기의 결과가 아닌 파일을 넘는 규칙 — 부모 순환, 위치, 참조 — 은 commit의 검사이며, T40이 key로 읽는다.
4. **copy-on-write post-image(T39).** 쓰기의 post-image는 읽은 corpus의 문서를 공유하고, 어떤 문서의 행을 처음 바꿀 때 복사한다.
   commit은 알림이 무엇을 말했든 자신의 target을 disk에서 읽는다. 서로에게서 읽거나 쓴 두 corpus는 모양이 key의 함수인, 문서의
   순서 있는 map의 경로를 따라 다른 곳에서만 비교하므로, `Differs`는 문서 수의 로그만큼 걷는다.
5. **key로 행 접근(T40).** 표의 저장된 행은 표를 읽지 않고 key로 읽는다. `placeBack`은 쓰기가 옮긴 형제 묶음을 배치하고,
   `allocate`는 corpus가 유지하는 index에서 prefix의 가장 큰 key를 가져온다. 그리고 commit이 대상 행의 key, 참조, 부모 순환, 위치,
   limit, unique column을 검사할 때는 표의 모든 행이 아니라, corpus가 문서별로 유지하고 corpus 사이에 공유하는 index로 필요한 행을
   읽는다.
6. **문장의 차이는 알려진다(T41).** database의 writer 연결은 수명 동안 쥐는 `*sql.Conn` 하나이고, 그 위에서 SQLite의 preupdate
   hook은 문장, 그 trigger, foreign key action이 건드린 행을 기록한다: 표, 연산, 예전과 새 rowid, 그리고 예전과 새 key. hook이 알리는
   key는 그 길이로 읽는다(`sqlite3_value_bytes`, release가 NUL에서 text 읽기를 멈추는 driver의 fork를 통해). text가 온전하지 않거나
   key 규칙(spec 20 Keys, `store.keyFault`)을 어기는 key는 쓰기를 만들기 전에 문장을 실패시킨다. driver와 그 fork는 Go이고, 빌드에 cgo는 필요 없다. temp schema는 무시하고, hook은
   fill, refresh, `placeBack` 동안 아무것도 기록하지 않는다. 행은 hook이 알린 순서로 최종 key에 따라 합쳐지며, 그 순서가 문장이
   insert한 행을 배치하는 순서다. 예전 행은 그 key의 corpus 저장 행이고(T40), 새 행은 `rowsOf`의 projection으로 key에 따라 읽는다.
   바뀐 key는 delete 하나와 insert 하나다. 각 문장은 writer transaction의 savepoint 아래에서 실행되고, callback이 기록한 실패는
   문장이 돌아올 때 그 savepoint로 되돌리고 문장을 실패시킨다. hook으로 만든 쓰기는 `diff`가 만드는 쓰기와 같으며, `diff`는
   trigger, CASCADE, REPLACE, UPSERT, WITHOUT ROWID, rowid 표, key 변경, 중간에 실패하는 문장에 대한 표 형식이고 fuzz된 차등 시험의
   oracle로 남는다.
7. **바뀐 행의 refresh(T42).** corpus가 다른 database는 자신의 세대 이후 commit들의 경로와 지난 refresh 이후 알림이 이름 댄 경로의
   행을 받고, 표의 availability가 바뀌었으면 그 표 전체를 받는다. 새 catalog는 새 database를 만든다. refresh는 fill처럼 transaction
   밖에서 foreign key를 끄고, 서 있는 trigger를 내리고, check를 무시한 채 실행된다. rowid로 key를 삼는 행은 저장된 rowid를 유지한다.
   다른 행의 rowid는 database 자신의 것이므로 `ORDER BY` 없는 `SELECT`에는 SQLite처럼 명시된 순서가 없다. AUTOINCREMENT counter는
   corpus의 일부다: 쓰기 뒤에 문장은 hook이 알리지 않는 `sqlite_sequence`를 AUTOINCREMENT 표마다 한 행씩 읽고, 오른 counter는
   commit에 들어가 `dbspec/sequences.json`에 쓰인다. fill과 refresh는 저장된 counter와 corpus의 가장 큰 key 중 큰 것을 가져오므로,
   손으로 쓴 key를 포함해 어떤 key도 두 번 주지 않으며, 이는 SQLite의 AUTOINCREMENT가 주지 않는 것과 같다. refresh한 database는 같은
   corpus로 처음부터 채운 database와 표마다, key에 따라, `sqlite_sequence`까지 같다.
8. **lock 없는 읽기(T43).** 읽기는 reader 연결에서 문장을 실행하고 실행하는 동안 corpus lock을 쥐지 않는다. cursor는 reader 연결을
   쥐고 lock은 쥐지 않는다. database는 읽기 전에 corpus에 맞춰진다: lock이 비어 있으면 읽기는 refresh에만 lock을 잡으므로 lock 기록은
   읽기마다가 아니라 변경마다 한 번 쓰인다. 살아 있는 holder가 lock을 쥐고 있으면 읽기는 lock 없이 refresh하며, 세대가 앞뒤로 같고
   어느 검사에도 prepare record가 없으면 일관되고, 그렇지 않으면 database의 세대로 답하고(spec이 이를 명시한다) 세대의 알림에 다시
   읽으며 timer로는 절대 읽지 않는다. 끝난 holder의 prepare된 stage는 읽기가 lock을 잡아 recover하며, 이는 SQLite reader가 hot
   journal을 되돌리는 것과 같고, spec은 읽기의 이 쓰기 하나를 명시한다. 읽기가 찾은 세대보다 뒤처진 database는 writer 연결이 비어
   있을 때 그것을 처음 찾은 읽기가 한 번 refresh한다. 지역 transaction이 writer 연결을 쥐고 있으면 다른 process는 commit할 수 없으므로
   읽기는 명시된 대로 database의 세대로 답한다. database는 `.flowmark/sql/<token>.db`에 있는 열기 전용 파일이고, 그 process의
   holder record(host, pid, 시작 시각)인 `<token>.holder` 옆에 있으므로 한 process의 두 열기는 database 둘을 가지며, `-wal`과 `-shm`을 가지고, SQLite의 자동 checkpoint(`wal_autocheckpoint` 명시)와 함께 WAL mode이며,
   writer 연결 하나와 명시된 크기의 reader 연결 pool을 가진다. pool이 찬 것을 본 읽기는 반환을 기다린다. 엔진은 `.flowmark/sql`을
   소유자만 접근하도록 만들고, 그것과 database 경로와 그 `-wal`, `-shm`을 열기 전에 lstat으로 검사해 link와, Windows에서는 reparse
   point를 거부한다. driver는 고정된 flag로 열며 `SQLITE_OPEN_NOFOLLOW`가 없고, 검사와 열기 사이에 link를 심으려면 그 folder에 대한
   소유자 자신의 접근이 필요하며, 이를 명시한다. 시작할 때의 recovery는 끝난 holder의
   pending lock 기록을 지우듯이 이 host에서 끝난 process의 파일을 지운다. 다른 host의 process 파일은 명시된 대로 절대 지우지 않는다.
   writer 연결은 문장부터 corpus commit까지 `BEGIN IMMEDIATE` transaction을 쥐고, corpus commit이 쓰인 뒤 commit하며, 모든 거부에서
   되돌린다. corpus commit이 쓰인 뒤 실패한 SQLite commit은 database를 다시 만들도록 표시한다. 지연 transaction은 첫 읽기부터 reader
   연결을 쥐고 세대를 비교해 upgrade한다. process가 읽기만 할 수 있는 corpus는 database를 memory에 두고 읽기를 하나씩 실행하며, 이를
   명시한다.
9. **한계는 명시된다(T44).** docs/spec/40-ops.md는 T36이 세는 비용과 benchmark가 1,000, 10,000, 100,000행에서 측정한 시간을 이
   경로의 한계로 명시한다.

각 행의 단위 시험은 촘촘하다: 행이 더하는 모든 규칙은 받아들이는 case와 거부하는 case를 표 형식으로 경계 case와 함께 가지며,
처음부터 만든 corpus나 database와의 각 동등성은 golden fixture를 seed로 하는 fuzz 시험이다. 행은 T36의 Red 수치가 green이 되고, 그
시험이 통과하고, kit의 lint와 gate가 통과하고, spec이 바뀐 것을 말할 때 끝난다.

**판정: commit은 원자성이 요구하는 만큼만 disk를 기다린다.** flush, 곧 파일이나 폴더의 sync와 Windows의 write-through move는
disk를 기다리며, Linux에서 약 1밀리초, macOS의 `F_FULLFSYNC`와 Windows runner에서 수십 밀리초가 걸리므로 commit의 지연은 읽기가
아니라 flush가 정한다. T31의 protocol은 stage 파일 하나와 이름 하나마다 따로 flush한다: 한 행 commit이 flush 18번을 하고, lock을
잡는 읽기가 lock record 때문에 한 번을 한다. SQLite는 `synchronous=FULL`에서 commit 하나에 한두 번을 한다. 아래 각 행은 TestCost의
flush cost counter로 재며, 그 수가 Red이다:

1. **flush를 기록한다 (T55).** journal은 disk에 요청한 flush를 자기 corpus의 counts에 더하고, write path가 그 counts를 함께 쓴다.
   기다리지 않는 corpus(`--synchronous off`)는 flush를 하지 않는다. TestCost가 모든 연산의 flush 수를 모든 크기에서 고정하므로, 경로에
   더해진 flush는 실패하는 수가 된다.
2. **move는 process가 기다릴 때만 기다린다 (T56).** Windows에서 `--synchronous off`는 다른 platform이 sync를 포기하듯
   `MOVEFILE_WRITE_THROUGH`를 포기한다.
3. **lock record는 flush하지 않는다 (T57).** lock은 link를 통해 온전한 채로 나타나므로 다른 process가 그 일부를 읽는 일은 없다. record가
   disk에 닿지 못한 lock은 정전만이 남기며, 그 정전이 holder를 끝냈다. 비었거나 0 byte만 담은 lock record는 그런 lock, 곧 끝난
   holder로 읽는다: 연산은 `flowmark unlock`을 밝혀 즉시 거부하고, unlock은 이전 boot의 holder가 남긴 lock을 지우듯 그것을 지운다. lock을
   잡는 읽기는 flush를 하지 않는다.
4. **record 하나가 commit을 정한다 (T58).** commit은 각 post-image의 사본을 쓰고 sync한 뒤, 모든 rename과 그 pre-hash와 post-hash,
   commit이 받는 generation, 모든 pre-image와 post-image의 byte를 담은 record 하나를 쓰고 그 폴더와 함께 sync한다: record부터 commit은
   정해진다. 이어 rename을 하고(delete는 대상을 journal 안으로 옮긴다) 대상의 폴더를 한 번씩 sync한다. 그다음 generation을 지금처럼 쓰고,
   record는 flush 없이 지운다: 정전이 되살린 record는 이미 쓰인 generation을 가리키므로 수거만 된다. T31의 commit record는 없어진다:
   generation이 rename이 끝났음을 말한다. 한 행 commit은 flush 여섯 번을 한다: 사본, record와 그 폴더, 대상의 폴더, generation과 그
   폴더. 이전 엔진의 stage 폴더는 그것을 완료할 엔진을 밝혀 거부한다.

행은 TestCost의 수가 명시한 수가 되고 protocol의 모든 단계에서 crash 시험이 통과할 때 끝난다.

**판정: 엔진은 흔한 database가 가진 것만 가진다.** flowmark는 orm이 MySQL, PostgreSQL, SQLite 옆에서 닿는 database이고,
orm의 단 하나의 규칙은 모든 database에서 같은 결과다. 그러므로 엔진의 기능은 그 database들이 가진 기능이고, markdown 고유의 문법은
database가 가진 값을 보여주기만 한다: database가 하지 않을 값을 주거나 검사하거나 기록하지 않는다. 흔한 database 어디에도 없는 기능은
제거하며, 상태기계가 처음이다(T62).

- **상태기계는 제거한다.** 흔한 database 어디에도 상태기계가 없다: column이 가질 수 있는 값은 그 type과 schema의 제약이고, 어떤
  database도 바뀌기 전 값 때문에 update를 거부하지 않는다. 엔진은 상태 column의 전이, terminal, 초기 상태, 필수 column, limit을
  강제하지 않고 이력 행을 쓰지 않는다. validator는 `history.gap`, `history.chain`, `state.transition`, `state.required`,
  `state.limit`을 보고하지 않는다. catalog는 기계를 두지 않고 `history`와 `initial`을 요구하지 않으며, 이력 표는 고유한 저장이나
  `markdown` 규칙이 없는 평범한 표다. `flowmark transition`(CLI, MCP, sidecar face의 `transition` 연산)과 `flowmark history repair`는
  없어지고, `schema` 연산은 기계를 답하지 않는다. 상태의 전이와 그 이력은 polyspec fsm의 것이다: fsm이 취한 상태는 다른 값처럼
  corpus에 쓰이고, 그 기록은 fsm이 간직한다. 이로써 §6의 상태기계 판정과 §9의 이력을 철회한다. 맥락의 불법 상태 전이는 fsm이 막는다.
- **남는 것은 값을 보여준다**: 행이 쓰이는 곳을 말하는 `store`와 그 `shard`, 행의 어느 자리가 column을 담는지 말하는 `title`,
  `body`, `markdown`, 그리고 목록 행이 column을 어떻게 보여주는지 말하는 `checkbox`다.
- **checkbox는 그 column의 값을 보여준다.** `checkbox <column>`은 목록으로 저장된 표의 column 하나를 이름 짓고, 목록 행은 그
  column을 task 표지의 괄호 사이에 쓴다. 다르게 저장된 표는 그 column을 다른 scalar를 쓰는 곳에 쓴다.
  - 한 글자 column `varchar(1)`은 `- [<value>] …`로 쓴다. 그 값은 모든 database가 제약하듯 schema가 허용하는 값 — 상태 표로의
    foreign key나 check `in (…)` — 이고, 제약 밖의 값은 모든 database가 거부하듯 `reference.dangling`이나 `check`로 거부한다.
    NULL은 상자 없는 목록 행 `- <key> …`이고, 빈 text는 `- [] …`이다. 한 줄 자리가 거부하는 글자는 여기서도 거부한다.
  - 값이 0과 1인 `bool` column은 0을 `- [ ] …`, 1을 `- [x] …`로 쓰며, GFM의 체크하지 않은 task 항목과 체크한 task 항목이다.
    GFM이 역시 체크로 읽는 `[X]`는 1로 읽고 `format.noncanonical`이다. NULL은 상자 없는 목록 행이다.
  근거: GFM task 항목은 괄호 사이의 한 글자이므로 한 글자 column은 그 값 전체를 거기 보여주고, boolean은 GFM 자신의 두 상자로
  보인다. 상태기계가 상태 이름마다 대응시킨 glyph는 각 값의 두 번째 표기였고, glyph가 없는 값은 쓸 수 없었는데, 그런 값을 거부하는
  database는 없다.
- **orm 문법.** 위의 checkbox 형태와 `state_machine` 제거는 orm의 문법(orm DBS-73, DBS-80, DBS-81, DBS-82, DBS-83과 DBS-63의
  반복 규칙)을 바꾸며, orm이 자기 행으로 추적한다. flowmark가 그것을 가진 orm release로 build하기 전까지 엔진은 orm이 내놓은 문법을
  읽되 기계는 하나도 강제하지 않는다: `state_machine` 줄을 모두 읽고 하나도 적용하지 않으며, 이는 DBS-73 자신이 database가 한다고
  말하는 것이자 MySQL, PostgreSQL, SQLite가 하는 것이다. orm이 기계의 상태 집합을 덮도록 요구하는 `checkbox <column> <state>
  '<glyph>'` 줄은 계속 목록 행의 각 값에 glyph를 주고, 어느 줄도 이름 짓지 않은 값의 목록 쓰기는 목록이 보여줄 수 없으므로
  거부한다. 기계가 checkbox를 선언하지 않은 목록 표는 그 column을 field로 쓴다. 그 orm release가 들어오면 flowmark는 자기 행으로 위의
  형태를 받고 상태별 형태를 거부한다.
- **`key_prefix`와 `order`는 값을 주므로** 같은 기준이 정한다.
  - `key_prefix '<p>'`는 text key `<p><n + 1>`을 할당한다. 흔한 database 어디도 key를 그렇게 할당하지 않는다: MySQL, PostgreSQL,
    SQLite는 AUTOINCREMENT, sequence, identity로 정수를 할당한다. orm은 그런 key를 뺀 insert를 corpus에서만 계획하므로(planner의
    `Assigns`) 같은 insert가 다른 모든 database에서는 `IR_INVALID`다. 제거한다: application이 key를 주거나, 표가 모든 database가
    가진 `identity`를 선언한다.
  - `order <column>`은 행을 형제 사이에 두고, 엔진은 insert에서 그 column을 할당하고 이동의 형제 번호를 다시 매기는데, 어떤
    database도 그러지 않으며 orm은 이것도 corpus에서만 계획한다. 할당과 재번호는 제거하고 application이 그 column을 다른 정수처럼
    쓴다. 값을 보여주는 것은 남는다: 형제 묶음의 행은 column의 순서로 쓰이므로 목록은 행의 순서를 보여준다. 자리는 값이 아니라 순위를
    보여주므로 값은 다른 scalar를 쓰는 곳 — 목록 행의 field, cell, 오늘 `files` 저장이 순위를 쓰듯 `files` 행의 항목 — 에 쓰고,
    값의 순서에서 벗어나 선 행은 `format.noncanonical`이며 `flowmark canon`이 다시 정렬한다.
  각각은 flowmark와 orm에서 자기 행으로 수행한다(orm DBS-77, DBS-79와 planner의 `Assigns`). 그때까지 엔진은 T60이 남긴 대로 둔다.

**T9 conformance 계약 (지금 확정)**: markdown vector는 corpus-before + IR request + 기대 corpus-after / 기대 rows이며 Go executor에 대해 실행한다. SQL/binds 비교는 적용되지 않는다(SQL이 없으므로). 다른 client가 executor를 port할 때 교차 비교가 시작된다. (Revision 3: vector는 flowmark 엔진에 대해 실행하고, orm의 executor conformance는 flowmark build를 고정한다.)

**이후 phase (checklist `[ ]` 항목으로만 등록)**: T2 codec(Go), T3 validator, T4 executor, T5 orm markdown-executor 통합(manifest setting, state-machine sub-grammar — orm 저장소 변경), T6 sync(adapter 계약과 database adapter; GitHub·Jira adapter는 그 아래에 등록), T7 세 개의 face — CLI·MCP server·soksak sidecar(protocol v1: stdio JSON lines, surface routing, host 소유 lifecycle; MCP tool 목록 mini-spec 먼저, lock은 작업마다 또는 server transaction마다 잡힌다(§5)), T8 checklist app(local issue model), T9 conformance vectors, T10 자기 repo의 soksak plugin — local 칸반 보드와 issue tracker, 또 하나의 thin consumer로서 view 전용 write path 없음.

## 감사가 부수지 못한 것

- 정합하는 두 flowmark writer 사이에서는 작업 단위 lock + stage/prepare/commit가 갱신을 잃지 않는다 (진짜 구멍은 reader, 사람, crash 창이었다 — 위에서 판정됨).
- dbspec 우선 파생 방향: 반대 방향은 이 계획이 거부하는 DDL-reconciliation conflict class를 실제로 만든다. (Revision 3은 방향을 유지하고 단독 fragment를 철회한다: catalog가 유일한 schema다.)
- corpus 위의 IR 표현력: 전 kind, relation(limit_per_parent, if_parent, flatten, key_by, 복합 key), tuple_in subquery, group_count, restore와 optimistic이 type-directed ordering과 PK순 결정성 아래 모호함 없는 in-memory 정의로 mapping된다.

## Open questions

open question은 `110-open-questions.md`에 있고, 결정되면 spec으로 승격한다.

## Verification (T1a)

- kit branch `feat/fifth-state-K13`: `node --test tests/kit/checklist.test.mjs` 13/13, `make documents-check` green (worktree에서 검증됨).
- flowmark `make kit-check`(lock 온전), `make documents-check`(쌍, doc-id, anchor, fence, source-sha256), 상태 5종 checklist 읽기 — green.
- `git log`(모든 저장소): conventional commit 형식, 기능 단위, 모든 `[o]` 항목에 changelog entry(flowmark) 또는 소유 row(kit) 동반.
- spec 완료 기준(구현 없이 검증 가능): 교차 참조 해석 가능, 개념당 이름 하나, 모든 판정이 근거를 동반한 명제, 2절 멱등성이 그렇게 명시됨.
- T1b가 `10-canonical-format`을 `[~]`로 시작할 수 있는 상태.
