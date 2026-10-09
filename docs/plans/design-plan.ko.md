<!-- doc-id: plans-design-plan -->
<!-- source-sha256: 34779daad8d89a8a23afb00bf5f68a69807e30d3a96981eb1b91c2fdc76bb0e6 -->
# flowmark 설계 계획

Revision 2 — 3개 lane의 적대 감사 결과(engine core, orm 통합, 운영)와 multi-platform 지시를 아래 판정에 접는다. 감사의 blocker는 전부 판정이 되었고, 부수지 못한 주장은 문서 끝에 남긴다.

## 배경

사용자는 markdown으로 engineering spec을 관리하며 checklist(상태 5종 `[ ]` `[~]` `[o]` `[!]` `[-]`, dotted 계층 ID, 선행/후속 의존), changelog, commit 규칙으로 개발을 운영한다. 문제: 문서 기반 운영에는 **멱등하고 DB 수준의 강제력이 없다** — 형식 위반, dangling foreign key, 불법 status 전환이 아무도 막지 못한다.

**flowmark의 정체성 (사용자 확정)**: **markdown 문법을 가진 텍스트 DB**. 파일 자체가 database이며 외부 DB 없이 standalone으로 완전 동작한다. **orm이 core**이고(markdown executor), **flowmark는 통합·응용 layer**다 — database, GitHub, Jira 등 tracker와의 양방향 동기화, 그리고 그 위에서 local 칸반 보드와 issue tracker(GUI는 또 하나의 thin consumer): GitHub과 Jira에도 보드는 있지만 여기서는 data를 로컬에서, git 안에서, 오프라인으로 관리한다. 핵심 가치: 저장 형식이 사람이 읽는 문서이고, git이 audit·branching·rollback을 제공한다.

**사용자 결정사항**:

1. polyspec/orm이 markdown을 **4번째 backend**(mysql/pgsql/sqlite/markdown)로 갖는다. CLI와 MCP는 orm model API가 **정공법 경로** (자체 write logic이 없는 thin adapter).
2. Schema 파생 방향: **dbspec ⇒ frontmatter** — dbspec이 root grammar이자 authority, frontmatter DDL은 dbspec의 embedding (상세 §4).
3. 문서는 **EN/KO 쌍을 day 1부터** (orm house style, kit 호환).
4. **polyspec/kit을 지금 vendor** — 5번째 상태 `[-]`는 **polyspec/kit 저장소에 upstream** 한다 (orm의 `scripts/kit`은 hash-lock된 vendor copy라 orm에서 직접 고쳐선 안 되고 `make kit-sync KIT_TAG=<tag>`로만 반영된다). 사용자가 kit에 대한 권한을 위임했다.
5. 이번 session의 deliverable = **설계 문서 + repository bootstrap** (구현은 checklist의 후속 phase로 추적). 감사의 범위 지적에 따라 spec 문서 자체는 다음 session(T1b)으로 분리하고, 이번 session은 T1a를 닫는다.
6. Sync projection 파일: 실제 markdown 파일이 disk에 존재. `readonly`(기본) / `writable` 옵션 (상세 §7).
7. Repository는 **github.com/polyspec/flowmark** — polyspec organization 안의 독립 repository.
8. **Multi-platform은 hard requirement. 편법 금지.** 현재 요구사항을 완전히 충족하는 가장 단순한 구현을 선택한다 (kit 자신의 규칙을 여기에 적용): flock 없음, POSIX 전용 durability 가정 없음 — lock은 kit의 `holder-lock` 선례(lock file + holder 신원 + stale 판정, 순수 runtime primitive)를 따르고, crash 안전성은 OS별 directory fsync가 아니라 journal replay로 얻는다.
9. **GUI는 자기 repo의 soksak plugin으로 출시되고, flowmark는 soksak의 sidecar로 서비스한다.** 하나의 Go binary가 세 개의 thin face을 갖는다 — CLI, MCP server, soksak sidecar(soksak core 명세의 protocol v1: 표준 입출력 위의 JSON lines, surface·root routing, host 소유 lifecycle). 저장소는 단일 언어 Go에 `package.json`과 `sidecar.json` manifest를 얹은 형태이고, 언어별 package는 요구사항이 이름을 붙일 때 만든다.

## 검증된 근거 (polyspec checkout에서 직접 확인)

- `orm/docs/protocol.md:12,96` — IR kind는 bounded: `one|all|count|group_count|sum|avg|paginate|insert|update|delete|restore`. request는 SQL text를 담지 않는다 → **IR request를 소비하는 native markdown executor는 bounded 문제**. 감사가 표현력을 확인했다: 전 kind와 Order/Pred/Func 표면이 in-memory corpus 위에서 계산 가능하며, 열린 문제는 명세였지 표현력이 아니었다.
- `orm/docs/dialects.md:184` — 3개 database가 collation을 binary codepoint order로 통일한다. 이 규칙은 **text 비교**에만 적용되고 typed key의 ordering에는 이전되지 않는다 (§2의 ordering 판정 참조).
- `kit/scripts/kit/documents-stamp.mjs`, `check-documents.mjs` — `source-sha256`는 **EN→KO 번역 sync marker**(Korean twin이 English 파일 raw bytes의 sha256를 보유). 데이터 drift 감지는 `.flowmark/` sidecar state로 설계하며, flowmark는 같은 marker에 **raw 파일 bytes hashing**을 쓴다 (kit 호환; 정규화는 hash가 아니라 검사에서 한다).
- `kit`은 독립 저장소(`polyspec/kit`)이며, consumer는 `make kit-sync KIT_TAG=<tag>`가 쓰고 `make kit-check`가 검증하는 `scripts/kit` + `tests/kit`의 hash-lock vendor copy를 가진다. vendored file은 kit에서만 바꾼다.
- `kit/scripts/kit/checklist.mjs` — 상태는 tracker별 data다. `[-]` 불가 상태는 이제 두 수준에서 읽힌다 (kit K13으로 upstream, branch `feat/fifth-state-K13` merge 대기 중).
- kit 문서 규칙: 모든 English 문서에 Korean twin 필요(doc-id 일치, explicit anchor 일치, fenced block 일치, twin이 source-sha256 보유), plans/와 CHANGELOG도 twin 검사 대상.

## 핵심 설계 (spec 문서가 확정할 내용)

### 1. Architecture와 seam

```
CLI / MCP / soksak sidecar (thin adapters — the orm model API is the orthodox path)
   ↓
polyspec/orm (.dbs → generated models → IR request)
   ↓ shared planner validates the IR (IR_INVALID rules unchanged)
markdown executor = the text database engine (standalone, consumes the IR request)
   ├─ codec: strict parser + canonical renderer (two-clause idempotency, §2)
   ├─ validator: corpus-global constraints (type/PK/UNIQUE/FK/CHECK/state machine)
   ├─ in-memory indexes derived from the parsed corpus (rebuilt on open, cached by content hash)
   ├─ transaction: stage → re-read validate → prepare record → renames → commit record
   └─ .flowmark/ sidecar: lock, tx journal, corpus revision, parsed-row cache
   ↓ optional
sync engine: md→db export / db→md materialize (§7)
```

**Seam 판정 (layering 순환 해소)**: markdown은 SQL executor들 옆에 등록되는 네 번째 **executor**이며 DSN scheme `markdown://<path>`로 선택된다. shared planner는 SQL backend와 똑같이 IR을 검증하고, markdown executor는 **IR request**를 직접 소비하며 SQL plan을 render하지 않는다 — `getQuery()`, statement event, `subscribe()`, `force_index`는 `CAPABILITY_UNSUPPORTED`를 반환한다 (지원 불가 능력에 대한 orm의 기존 선례). executor 구현은 **orm 저장소**에 귀속된다 (flowmark가 T5에서 기여한다). 다른 client가 port하기 전까지 capability table은 markdown 지원을 **Go-only**로 표시한다. flowmark 저장소는 spec, kit tooling, checklist application, sync engine을 담는다.

- SQLite mirror는 부정한다: mirror↔corpus 동등성이 영구적인 bug class가 된다. mirror를 인정하더라도 conformance 조건은 (a) full-rebuild equivalence (b) write path 부재다.
- IR boundedness 주장은 request layer만 대상으로 한다. **API 동등성은 주장하지 않는다** — capability table(§6)이 계약이며, orm의 `CAPABILITY_UNSUPPORTED`/`OPERATOR_NOT_ALLOWED` 전통을 따른다.

### 2. Canonical format (`10-canonical-format.md`)

- **멱등성은 dbspec의 2절 명제다** — 단일 byte-identity 주장이 아니라: canonical 입력 `s`에 대해 `emit(parse(s)) == s`, 임의의 valid 입력 `d`에 대해 `emit(parse(emit(d))) == emit(d)`. renderer는 **구조 skeleton** — frontmatter block, heading line, 정렬 대상인 item/table line, renderer가 쓰는 marker — 만 소유하고, 보존 구간의 payload bytes(prose, fenced 내용, 소유하지 않는 HTML comment)는 결코 다시 쓰지 않는다. 비-canonical 입력은 `format.noncanonical` diagnostic과 별도의 일회성 `flowmark canon` migration transaction으로 처리하며, 논리적 write의 부수 효과로 정규화하지 않는다.
- 문서 preamble(첫 H2 이전의 제목과 text)는 예약된 long-text column이다. H2 section은 선언된 long-text column에 mapping되고, 없는 section = NULL, 있으나 비어 있음 = 빈 text다.
- **Ordering은 type-directed**: ID-typed key는 segment별 숫자 비교(`T8.0.9 < T8.0.10`), text는 codepoint, 복합 key는 component type별 사전식. SQL collation 결정은 text 비교에만 적용되며 typed key의 ordering으로 수입하지 않는다. `group_count`는 NULL을 같은 group으로 처리한다 (SQL 동등성).
- ID grammar: `T<n>(.<n>)*(-<n>)?` — leading zero와 suffix `-0` 금지; 동등성은 canonical 형식의 문자열 동등성; suffix는 항목 신원의 일부로 **renumber 때 base와 함께 이동**한다. suffix 항목의 하위(`T8.0.14-1.3`)는 허용하며 suffix는 마지막 path segment에 결합한다. 참조는 구조적으로 읽는다(parse된 위치, code span 제외); 참조를 다시 쓰는 codemod가 payload 보존의 명시적 예외다. `flowmark next-id`가 다음 형제 ID를 계산한다 — 사람이 도구 없이 ID를 계산한다고 기대하지 않는다.
- **Frontmatter profile**: YAML 1.2 core schema, 값 영역은 block style, anchor·tag 금지. v1에서 frontmatter comment는 located diagnostic으로 거부한다 (comment를 보존하는 round-trip emitter는 요구사항이 명명하기 전까지 명시된 비목표). dbspec type별 값 문법: decimal·datetime·uuid literal은 dbspec의 canonical 형식, `bytes`는 base64, 줄바꿈을 가질 수 있는 text column은 markdown-table child cell에 둘 수 없다.
- 번역 쌍은 **하나의 row**다: `doc_id`가 primary key이고 Korean twin 파일은 row의 projection이지 두 번째 row가 아니다. `source-sha256` = English 파일 raw bytes의 sha256 (kit 호환).

### 3. Document-table model (`20-document-model.md`)

- Folder = table, file = row, H2 section = long-text column, 구조화 body block(checklist 항목, markdown table) = child table (FK).
- Frontmatter = row data + `table` binding + `schema_version` stamp (+ standalone인 경우 embed된 DDL).

### 4. Schema (`30-schema.md`) — 파생 판정: dbspec ⇒ frontmatter

- **dbspec이 root grammar이자 authority.** frontmatter DDL은 독립 언어가 아니라 dbspec grammar의 frontmatter embedding이다: fragment는 항상 **YAML literal block scalar**(`|`)이며, 이것이 YAML-적대적 token(`yes`/`on`/`null` 같은 이름, datetime의 colon, fragment 안의 `#`)을 해결하고 문법·parser·type system을 하나로 유지한다. dbspec diagnostic은 file 좌표로 offset-mapping한다. fragment는 embed 시점에 canonicalize하고(LF, 마지막 개행), stamp는 **canonical** fragment를 담는다 — 두 번째 minified 출력 형식은 없다.
- 방향의 근거: (1) schema는 law, data는 row — 문서별 DDL은 law의 사본이며 사본이 어긋나는 것 자체가 drift bug class다 (2) orm의 engine/IR/conformance가 dbspec manifest hash에 묶여 있어 두 번째 grammar는 부품을 두 벌로 만든다 (3) dbspec의 parser와 type system을 재사용한다 (4) 반대 방향은 corpus-global schema를 문서별 선언의 merge로 구성해 branch 간 DDL reconciliation이라는 새 conflict class를 만든다.
- **Standalone 문서**(repo 밖 단일 파일): embed된 fragment가 그 문서의 임시 authority이며 하나의 문서 집합(1-document set, `use` 선언 없음) 위에서 같은 IR로 compile된다. corpus로 import되면 repo의 .dbs가 authority가 되고 fragment는 정합성 검사 대상이 된다.
- **자기 서술성은 projection으로 충족**: schema stamp(canonical fragment + manifest hash + `schema_version`)은 .dbs에서 frontmatter로 render된다 — data가 아니라 verified cache이며, 불일치는 located `schema.drift` diagnostic이지 silent reparse가 아니다.
- **Markdown 고유 schema 표면은 manifest-level setting이지 render되는 type이 아니다** (introspection 정합성, `updated`/`soft_delete` 선례): markdown 성질의 column trait와 checklist child-table sugar는 setting이며, 실제 database는 이들을 평범한 `text`로 introspect한다. state machine도 같은 방식으로 자체 sub-grammar를 가진 manifest-level setting이다(`audit`처럼) — SQL backend는 `navigation`처럼 무시하고 markdown validator가 강제한다.
- **`identity` column은 markdown corpus에서 거부된다.** key는 사람이 부여한다(ID grammar); primary key를 생략한 insert는 error이고 `flowmark next-id`가 할당 보조다. git 되감기에서의 key 비재사용은 설계상 포기한다 — 할당 기록은 sequence가 아니라 git history다.

### 5. Ops와 transaction (`40-ops.md`)

- **Commit protocol** (순서가 규범적): post-image 파일을 `.flowmark/tx/<id>/`에 content-addressed로 쓰고 fsync; 모든 대상의 pre-image를 stage로 복사하고 fsync; 모든 rename과 pre/post hash를 적은 **prepare record**를 쓰고 fsync; runtime의 atomic rename으로 rename; commit record를 쓰고 fsync; stage 정리. **복구는 corpus를 열 때마다 실행한다**: prepare record가 있으면 완료, 없으면 stage 폐기, 대상의 현재 hash가 기록된 어느 쪽과도 다르면 거부하고 보고한다(crash 사이에 사람 편집이 들어왔다는 뜻 — 결코 조용히 덮어쓰지 않는다). 정전 이후의 durability는 OS별 directory fsync가 아니라 journal replay에 의존한다.
- **하나의 portable lock, 작업 단위** (multi-platform 판정): 모든 작업 — 읽기와 쓰기, CLI와 MCP — 는 같은 lock file을 작업 기간 동안 holder 신원·stale 판정과 함께 획득한다(kit `holder-lock` 선례) — timeout은 holder를 보고한다. 장기 lock도 reader/writer 구분도 없다: reader가 torn commit을 볼 수 없고, MCP server가 CLI를 잠그지 않으며, 멈춘 holder는 deadlock이 아니라 timeout 보고다. commit 시 충돌(읽은 뒤 대상이 바뀜)은 file과 line과 함께 실패하고 정의된 재독기-재시도-1회 계약을 따른다 — silent merge 없음.
- **검증은 commit 시점에 lock 아래에서 corpus를 새로 읽어 수행한다** — BEGIN snapshot도 예측된 touch file 집합도 아니다(cascade 탐색이 그 집합을 계산 불가능하게 만든다). read-hash는 실제로 읽은 것에서 계산한다.
- **Corpus revision**: `.flowmark/`의 단조 증가 counter이며 모든 commit이 올린다. 다중 호출 read session(paginate)은 이것을 가지고 다니며 불일치 시 `CORPUS_CHANGED`로 실패한다(또는 keyset pagination으로 강등). `orderByRandom`은 revision별 seed.
- **비용 모델**: 40-ops가 per-command p95 latency 목표를 선언한다(corpus envelope 기준 수백 ms 단위); file content hash를 key로 하는 `.flowmark/cache/`의 parsed row cache가 변하지 않은 file의 재parse를 피한다; 기본 per-transaction 검증은 post-image의 FK/UK closure이고, 전체 `flowmark validate`는 on demand와 schema 변경 시에 실행한다.
- Walk 규칙: `.flowmark/`와 숨김 항목 건너뛰기; corpus 안의 symlink·hardlink는 located diagnostic; nested corpus root는 error.

### 6. Constraints와 capability table (`50-constraints.md`, `60-dialect.md`)

- Strict parse, type/PK/UNIQUE/FK(dangling 선행 거부, NFC 후 exact match), CHECK. soft-delete된 row는 unique key를 계속 차지하고 FK를 계속 만족한다(SQL 동등성 — soft delete가 가짜 판정을 일으키지 않게 명시). AES codec column을 선언한 corpus의 검증은 key를 요구하며, key 없는 실행은 근사하지 않고 실패한다.
- **state machine은 dbspec-level manifest setting** (전환 table + terminal + 필수 field)이다; checklist app은 그 위에 `[o]` terminal→supersede(`T8.0.14-1`), WIP gate, `[!]` 원인/재시도와 `[-]` 원인 필수를 인코딩한다. diagnostic 형식은 kit와 동일: `{line, column, rule, message}`.
- **사람의 write는 DB의 writer 독점 전제를 무효화한다; 각 기능은 그에 대한 판정을 갖는다**: FK action은 markdown corpus에서 **restrict만 허용**(`cascade`/`set_null`은 schema parse에서 거부 — executor를 우회하는 writer에게 구현 불가능하므로); `audit`와 `immutable`은 advisory다(validate 시점 거부, 사람 편집에 대한 trigger 시점 예방 없음); non-null 기본값 column이 빠진 사람 작성 파일은 validate가 거부하고 `flowmark ingest`가 기본값을 명시적으로 찍는다.
- **Capability table은 T1b에서 전체 내용으로 작성한다** (그것이 통합 계약이다): lock 4종 거부(또는 `update|share`를 문서화된 no-op로 받고 `*_nowait` 거부); `isolation`/`timeoutMs`, `fulltext`/`Lb` 거부; `on_duplicate` 충돌 대상을 명시적으로 정의(규칙을 쓰고, 다중 unique key table에서 형제 backend와 갈라지는 지점을 문서화); 평범한 `update`가 markdown에서 optimistic 의미로 강화됨을 명시(모든 commit이 재독기); `getQuery`, statement event, `subscribe`, `force_index`는 `CAPABILITY_UNSUPPORTED`; savepoint는 nested staging으로 mapping.
- markdown plan lifecycle(orm의 dbplan/apply/recover 장치)은 **v1 범위 밖**: v1은 corpus 생성과 additive column 추가를 지원하고, markdown corpus의 dbplan chain은 거부한다(80-migration이 범위와 이유를 서술).

### 7. Sync와 adapter (옵션, `70-sync.md`)

- sync engine은 **하나의 계약을 가진 adapter architecture**다; database가 첫 adapter이고 GitHub(issues/projects)과 Jira가 따른다: table별 truth ownership, 소유 system을 밝히는 provenance header(`<!-- owner: <adapter>:<table>@<자기 bytes의 sha256> -->`), git을 인식하는 drift 처리, merge base로 기록되는 materialized-from revision, 명시적 일회성 command(live daemon 없음). 외부 tracker의 항목은 corpus row로 materialize된다 — checklist app의 item model이 곧 local issue model이므로 local board와 remote tracker가 같은 row의 두 projection이다.
- md→db: derived export (재구축 가능). db→md(그리고 모든 adapter→md): materialize — hash는 materialize된 파일 자체 bytes의 것이므로 검사가 자기 검증되고 clone에 안전하며, 그 범위를 정직하게 명시한다: 사고 방지이지 공격자 방지가 아니다. validator는 adapter-owned 문서의 편집을 거부한다(readonly 기본); `writable`은 검증 후 re-ingest한다.
- **readonly drift는 git을 인식한다**: working tree가 HEAD와 다르면 HEAD로 복구하고 보고; tree가 깨끗한데 HEAD가 마지막 ingested version과 다르면 수용하고 re-baseline한다. adapter-owned path의 merge conflict는 양쪽을 버리고 re-materialize로 해결한다 — 그곳의 손 merge는 diagnostic이다.
- changelog projection은 `item_revisions` — 파일 안의 data — 에서 유도하며 gitignore된 sidecar journal에서 절대 유도하지 않는다(fresh clone이 모든 projection을 재구축할 수 있어야 한다).

### 8. Migration (`80-migration.md`)

- v1 범위: .dbs에서 corpus 생성, additive column 추가, 명시적 transactional codemod(`flowmark renumber`는 항목과 모든 구조적 참조를 한 transaction에 다시 쓴다 — suffix는 base와 함께 이동 — dangling이나 충돌이 생기면 거부). additive-first; rename은 명시적 step; dbplan chain은 거부(§6 참조).

### 9. Checklist app (`100-checklist.md`)

- tables: `checklists`, `checklist_items`(self-FK parent, 상태 5종, supersedes FK), `item_dependencies`(선행/후속), `item_revisions`(append-only: from/to, reason, evidence), `changelogs`(item_revisions의 projection).
- **View는 또 하나의 thin consumer일 뿐**: 칸반 보드(column = 상태 5종, card = checklist 항목, 의존은 edge)는 같은 orm query로 읽고 같은 orm model API로 transition을 쓴다 — 불법 drag는 불법 CLI transition과 똑같이 validator가 거부하고 WIP gate는 column limit이 된다. view 전용 write path는 존재해선 안 된다 (후속 phase 항목으로 등록, 같은 원칙은 향후 어떤 UI에도 적용).
- kit→flowmark checklist cutover(app 출시 시)는 하나의 transaction이다: `config/checklist.json`에서 tracker를 빼고, 문서를 corpus 형식으로 codemod하고, 같은 commit에서 `flowmark validate`를 통과시킨다. 그 전까지는 stopgap reference checker(docs/의 모든 `T<n>…` token이 checklist 항목으로 해석되는지)가 kit가 검사할 수 없는 dangling-reference 계열을 검사한다.

## Session 계획 (이번 session은 T1a, 다음 session은 T1b)

**T1a — 감사 반영 설계 + bootstrap** (이번 session)

1. flowmark `git init`, `.gitignore`(`.flowmark/`), `README.md`+`README.ko.md`, `AGENTS.md`+`AGENTS.ko.md`(운영 규칙: checklist discipline, conventional commits, [o]↔changelog↔commit 단위, twin workflow — 한국어에서 발견한 수정은 다음 stamp 전에 English 파일로 이식, 문서 commit에 대한 interactive history 편집 금지), `CHANGELOG.md`+`CHANGELOG.ko.md`(`## Unreleased`). commit: `chore: Bootstrap flowmark repository`.
2. **kit `[-]` upstream** — branch `feat/fifth-state-K13`에서 완료(worktree `kit-fifth-state-K13`, red vector → FIVE_STATES + 원인 규칙 → green, K13 row + AGENTS 쌍 + stamp된 KO checklist). kit의 병렬 작업이 안정화되면 `main`에 merge하고 worktree와 branch를 즉시 제거한다. orm은 사용자가 kit release에 tag를 붙일 때 `make kit-sync`로 채택한다.
3. flowmark에 **kit 디렉터리 전체 vendor**(`scripts/kit` + `tests/kit` + `kit.json` + `.kit/kit.lock.json`, kit-sync 계약 준수, lock은 kit commit을 기록)하고 `config/checklist.json`(list format, `T<n>(.<n>)*(-<n>)?` ID pattern, 상태 5종, active `[~]`)과 `config/documents.json`을 작성한다. commit: `chore: Vendor polyspec/kit tooling`.
4. `docs/plans/execution-checklist.md`+`.ko.md` — 사용자 형식(doc-id, 선행/후속, 원인/완료 기준/Evidence)으로 T1a/T1b/T2–T9 항목을 담고 전 과정에서 갱신한다.
5. 이 설계 계획(revision 2)과 Korean twin — 감사를 접은 상태로 commit.

**T1b — spec 문서** (다음 session; EN/KO 쌍, 문서당 commit 1개, twin stamp): `00-overview`, `10-canonical-format`(§2), `20-document-model`(§3), `30-schema`(§4), `40-ops`(§5), `50-constraints`(§6), 그리고 skeleton `60-dialect`(capability table을 전체 내용으로), `70-sync`, `80-migration`, `90-conformance`, 전체 내용 `100-checklist`, `110-open-questions`. 문서별 완료 기준: 모든 판정이 근거를 동반한 명제로 나타난다 (prose 분량이 아니라 결정 수).

**T9 conformance 계약 (지금 확정)**: markdown vector는 corpus-before + IR request + 기대 corpus-after / 기대 rows이며 Go executor에 대해 실행한다. SQL/binds 비교는 적용되지 않는다(SQL이 없으므로). 다른 client가 executor를 port할 때 교차 비교가 시작된다.

**이후 phase (checklist `[ ]` 항목으로만 등록)**: T2 codec(Go), T3 validator, T4 executor, T5 orm markdown-executor 통합(manifest setting, state-machine sub-grammar — orm 저장소 변경), T6 sync(adapter 계약과 database adapter; GitHub·Jira adapter는 그 아래에 등록), T7 세 개의 face — CLI·MCP server·soksak sidecar(protocol v1: stdio JSON lines, surface routing, host 소유 lifecycle; MCP tool 목록 mini-spec 먼저, 작업 단위 lock은 이미 core 판정), T8 checklist app(local issue model), T9 conformance vectors, T10 자기 repo의 soksak plugin — local 칸반 보드와 issue tracker, 또 하나의 thin consumer로서 view 전용 write path 없음.

## 감사가 부수지 못한 것

- 정합하는 두 flowmark writer 사이에서는 작업 단위 lock + stage/prepare/commit가 갱신을 잃지 않는다 (진짜 구멍은 reader, 사람, crash 창이었다 — 위에서 판정됨).
- dbspec ⇒ frontmatter 파생 방향: standalone fragment의 임시 authority가 오프라인 문서를 덮고, 반대 방향은 이 계획이 거부하는 DDL-reconciliation conflict class를 실제로 만든다.
- corpus 위의 IR 표현력: 전 kind, relation(limit_per_parent, if_parent, flatten, key_by, 복합 key), tuple_in subquery, group_count, restore와 optimistic이 type-directed ordering과 PK순 결정성 아래 모호함 없는 in-memory 정의로 mapping된다.

## Open questions (`110-open-questions.md`에 보관, 결정 시 spec으로 승격)

- flowmark 자체 license — 권고: Apache-2.0 (patent grant; AGPL 이웃 옆의 schema-language 파생물). 첫 공개 push 전에 결정.
- corpus 규모 envelope(예: 문서 ≤10k / child row ≤100k)과 40-ops가 선언할 latency 예산 숫자.
- `T8.0.14-1.3` suffix 하위 항목 판정 확정 (권고: 허용 — 10-canonical-format에 명제로 재서술).
- frontmatter comment 지원 (v1 거부; 요구사항이 명명하면 재검토).
- MCP tool 목록과 호출별 표면 (T7 mini-spec).
- manifest setting 확장의 orm 측 반영 순서 (T5).

## Verification (T1a)

- kit branch `feat/fifth-state-K13`: `node --test tests/kit/checklist.test.mjs` 13/13, `make documents-check` green (worktree에서 검증됨).
- flowmark `make kit-check`(lock 온전), `make documents-check`(쌍, doc-id, anchor, fence, source-sha256), 상태 5종 checklist 읽기 — green.
- `git log`(모든 저장소): conventional commit 형식, 기능 단위, 모든 `[o]` 항목에 changelog entry(flowmark) 또는 소유 row(kit) 동반.
- spec 완료 기준(구현 없이 검증 가능): 교차 참조 해석 가능, 개념당 이름 하나, 모든 판정이 근거를 동반한 명제, 2절 멱등성이 그렇게 명시됨.
- T1b가 `10-canonical-format`을 `[~]`로 시작할 수 있는 상태.
