# items
- [x] T1 Ship the parser position:1 owner:[P1](people.flowmark.md#people-row-p1) estimate:12.50 done:true score:0.5 seen_on:2026-10-01 seen_at:"2026-10-01 09:30:00" clock:08:00:00.250 ref:0f8fad5b-d9cb-469f-a165-000000000001 blob:aGVsbG8= weight:-3 big:9000000000 note:""
  - [~] T2 Write the tests position:1 done:false
- [ ] T3 Review the design position:2 done:false note:"say \"hi\" \\ bye"
- [ ] T4 Set owner\: review done:false

## items row T1

The parser reads the list rows from the left and the fields from the right.

### evidence

Run the golden tests.

### item_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 1 | waiting | doing | 2026-10-01 08:00:00.000000 |
| 2 | doing | done | 2026-10-01 09:00:00.000000 |

#### item_history row 2

##### evidence

Run the golden tests.

### item_notes
- N1 Spec the codec position:1 tag:urgent
  - N2 Child note position:1 tag:""
- N3 Plain position:2

#### item_notes row N1

Body of N1.

## items row T2

The tests are fuzzed.

### item_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 3 | waiting | doing | 2026-10-02 08:00:00.000000 |

## items row T4

### cause

### item_checks
- [x] C1 Plan the position:1 owner:[P1](people.flowmark.md#people-row-p1) estimate:12.50 done:true score:0.5 seen_on:2026-10-01 seen_at:"2026-10-01 09:30:00" clock:08:00:00.250 ref:0f8fad5b-d9cb-469f-a165-000000000001 blob:aGVsbG8= weight:-3 big:9000000000 note:""
  - [~] C2 Draft position:1 done:false
- [ ] C3 Review position:2 done:false note:"say \"hi\" \\ bye"

#### item_checks row C1

Body of C1.

##### evidence

Proof.

##### item_check_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 1 | open | doing | 2026-10-01 08:00:00.000000 |
| 2 | doing | done | 2026-10-01 09:00:00.000000 |

#### item_checks row C2

##### item_check_history
| id | from_state | to_state | at |
| --- | --- | --- | --- |
| 3 | open | doing | 2026-10-02 08:00:00.000000 |
