<!-- doc-id: checklist-kit-standard -->
<!-- source-sha256: 1691112e6564f6e88fce9f919e592ed7fb9acd29fb22a3c810d35ba8f17d38f5 -->
# kit 표준 체크리스트

| ID | Task | Deliverables | Verification | State |
|---|---|---|---|---|
| T15 | 저장소가 kit 표준 0.0.54를 따른다 | flowmark follows the kit standard 0.0.54: its layout, its plans, its checks and its gates | `make kit-check` | [o] |
| T15-1 | kit gate의 발견을 고친다: Go lint와 skip, CI와 composer package | golangci and deadcode pass in packages/go; no test skips; CI runs make flowmark-test and make refs-check; composer packages are published releases; knip declares its entries and binaries | `node --test tests/*.test.mts` | [!] 원인: 타입 인식 TypeScript lint가 아직 실패한다: scripts/record-slugs.mts에 타입이 없고 board 예제가 어떤 install도 제공하지 않는 package를 import한다; 재시도: record-slugs.mts와 board 예제에 타입을 붙여 make lint-ts가 통과할 때 |
