<!-- doc-id: checklist-kit-standard -->
<!-- source-sha256: 5f23fcc1f86df29dd67a22459e3aff5048e71cd098365fdccca98daad7df5458 -->
# kit 표준 체크리스트

| ID | Task | Deliverables | Verification | State |
|---|---|---|---|---|
| T15 | 저장소가 kit 표준 0.0.54를 따른다 | flowmark follows the kit standard 0.0.54: its layout, its plans, its checks and its gates | `make kit-check` | [o] |
| T15-1 | kit gate의 발견을 고친다: Go lint와 skip, CI와 composer package | golangci and deadcode pass in packages/go; no test skips; CI runs make flowmark-test and make refs-check; composer packages are published releases; knip declares its entries and binaries | `node --test tests/*.test.mts` | [!] 원인: 타입 인식 TypeScript lint가 아직 실패한다: scripts/record-slugs.mts에 타입이 없고 board 예제가 어떤 install도 제공하지 않는 package를 import한다; 재시도: record-slugs.mts와 board 예제에 타입을 붙여 make lint-ts가 통과할 때 |
| T15-2 | config/parity.json의 모든 unit은 모든 구현에서 성립하고, test가 이름으로 적은 spec requirement를 인용한다 (kit K273, K275, K276) | config/parity.json, docs/spec, the tests of every implementation | `make parity-declarations-check` | [!] 원인: 선언이 아직 profile, native 전용 symbol이나 exclusion, 또는 명명된 requirement가 없는 unit을 담고 있다; 각 구현의 누락된 feature와 test는 red-first로 작성되어야 한다; 재시도: kit 0.0.55를 make kit-sync로 설치한 후 |
| T15-3 | commit-msg-check는 check-commits를 $(KIT_EXT)로 실행한다(kit K613). 원인: target이 kit의 dist 파일인 check-commits.mjs를 가리켰는데, kit이 source를 실행하면 저장소는 그 파일을 더 실행하지 않는다. 수정: recipe는 $(KIT)/check-commits.$(KIT_EXT)를 가리킨다 | Makefile | `make -n commit-msg-check MESSAGE=x` | [o] |
