# items
| id | parent | state | title | assignee | start_on | start_at | due_on | due_at | priority |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| T1 |  | done | 디자인 시안 확인 | [P1](people.flowmark.md) |  |  | 2026-10-08 |  |  |
| T2 |  | doing | API endpoint 목록 작성 | [P2](people.flowmark.md) |  |  |  | 2026-10-11 21:00:00 |  |
| T3 |  | waiting | 라우팅 테스트 통과 |  | 2026-10-12 |  | 2026-10-16 |  | 1 |
| T4 | [T3](#items-row-t3) | doing | 회귀 테스트 작성 | [P1](people.flowmark.md) |  |  |  |  | 2 |
| T5 |  | bypassed | 변환 스크립트 작성 | [P2](people.flowmark.md) |  | 2026-10-06 10:00:00 | 2026-10-14 |  |  |
| T6 |  | impossible | 구 버전 API 유지 |  |  |  |  |  |  |

## items row T1

Figma의 최신 시안을 확인하고 색상 팔레트와 타이포그래피를 검토한다.

### evidence

시안 검토를 마쳤다.

### item_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 1 | waiting | doing | 2026-10-02 10:00:00.000000 |
| 2 | doing | done | 2026-10-03 11:00:00.000000 |

#### item_history row 2

##### evidence

시안 검토를 마쳤다.

## items row T2

모든 REST endpoint를 정리하고 request/response schema를 문서화한다.

### item_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 3 | waiting | doing | 2026-10-04 09:30:00.000000 |

### item_links
| id | to_item | kind |
| --- | --- | --- |
| 1 | [T3](#items-row-t3) | blocks |

## items row T3

모든 route에 대한 통합 테스트가 통과해야 한다.

## items row T4

### item_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 4 | waiting | doing | 2026-10-05 14:00:00.000000 |

## items row T5

기존 주문 데이터를 새 schema로 옮기는 script를 작성한다.

### cause

원본 주문 dump가 아직 없다.

### retry

dump가 도착하면 다시 시작한다.

### item_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 5 | waiting | doing | 2026-10-06 10:00:00.000000 |
| 6 | doing | bypassed | 2026-10-07 16:00:00.000000 |

#### item_history row 6

##### cause

원본 주문 dump가 아직 없다.

##### retry

dump가 도착하면 다시 시작한다.

### item_links
| id | to_item | kind |
| --- | --- | --- |
| 2 | [T2](#items-row-t2) | relates |

## items row T6

### cause

하위 호환은 두지 않는다.

### item_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 7 | waiting | impossible | 2026-10-08 09:00:00.000000 |

#### item_history row 7

##### cause

하위 호환은 두지 않는다.
