Each cell: ✓/✗ correct · input tokens · output tokens (reasoning) · latency · start of the reply. ⚠ marks questions where the arms disagree on correctness. Full replies are in the JSONL records.

## nim:openai/gpt-oss-20b

| | question ID | data | kind | question | gold | json-compact | gateway |
|---|---|---|---|---|---|---|---|
|  | orders-30-s1000-q1 | orders/30/s1000 | lookup | What is the customer name on order id 10029? | Initech Foods | ✓ · 1628 · 46 (33) · 2.3s · Initech Foods | ✓ · 1343 · 80 (67) · 3.7s · Initech Foods |
|  | orders-30-s1000-q2 | orders/30/s1000 | lookup | What is the amount of order id 10027? | 1249.62 | ✓ · 1627 · 76 (62) · 1.9s · 1249.62 | ✓ · 1342 · 80 (66) · 1.7s · 1249.62 |
|  | orders-30-s1000-q3 | orders/30/s1000 | count | How many orders have status "paid"? | 9 | ✓ · 1624 · 166 (155) · 6.1s · 9 | ✓ · 1339 · 174 (163) · 4.2s · 9 |
|  | orders-30-s1000-q5 | orders/30/s1000 | argmax | Which order id has the largest amount? | 10024 | ✓ · 1624 · 237 (225) · 7.4s · 10024 | ✓ · 1339 · 89 (77) · 2.6s · 10024 |
|  | orders-30-s1000-q4 | orders/30/s1000 | count | How many orders have status "paid" and currency "EUR"? | 3 | ✓ · 1629 · 355 (344) · 9.1s · 3 | ✓ · 1344 · 437 (426) · 14.8s · 3 |
|  | orders-30-s1000-q7 | orders/30/s1000 | lookup | What is the created_at timestamp of order id 10004? | 2026-04-15T05:49:00Z | ✓ · 1629 · 65 (42) · 1.0s · 2026-04-15T05:49:00Z | ✓ · 1344 · 99 (76) · 5.0s · 2026-04-15T05:49:00Z |
|  | orders-30-s1000-q6 | orders/30/s1000 | sum | What is the total number of items across all orders from customer "Oscorp Corp"? | 36 | ✓ · 1633 · 4928 (4917) · 120.6s · 36 | ✓ · 1348 · 511 (500) · 6.2s · 36 |
|  | orders-120-s1000-q1 | orders/120/s1000 | lookup | What is the customer name on order id 10117? | Oscorp Systems | ✓ · 5878 · 55 (42) · 1.8s · Oscorp Systems | ✓ · 4397 · 80 (67) · 2.1s · Oscorp Systems |
|  | orders-120-s1000-q2 | orders/120/s1000 | lookup | What is the amount of order id 10070? | 1459.81 | ✓ · 5877 · 43 (29) · 1.8s · 1459.81 | ✓ · 4396 · 83 (69) · 1.0s · 1459.81 |
|  | orders-120-s1000-q3 | orders/120/s1000 | count | How many orders have status "paid"? | 24 | ✓ · 5874 · 816 (805) · 12.1s · 24 | ✓ · 4393 · 1225 (1214) · 40.6s · 24 |
|  | orders-120-s1000-q4 | orders/120/s1000 | count | How many orders have status "paid" and currency "EUR"? | 8 | ✓ · 5879 · 802 (791) · 26.8s · 8 | ✓ · 4398 · 2152 (2141) · 36.4s · 8 |
|  | orders-120-s1000-q6 | orders/120/s1000 | sum | What is the total number of items across all orders from customer "Wayne Logistics"? | 12 | ✓ · 5883 · 107 (96) · 2.9s · 12 | ✓ · 4402 · 189 (178) · 4.8s · 12 |
|  | orders-120-s1000-q5 | orders/120/s1000 | argmax | Which order id has the largest amount? | 10079 | ✓ · 5874 · 1329 (1317) · 46.1s · 10079 | ✓ · 4393 · 2594 (2582) · 26.9s · 10079 |
|  | orders-120-s1000-q7 | orders/120/s1000 | lookup | What is the created_at timestamp of order id 10099? | 2026-03-11T21:04:00Z | ✓ · 5879 · 67 (44) · 2.0s · 2026-03-11T21:04:00Z | ✓ · 4398 · 94 (71) · 3.1s · 2026-03-11T21:04:00Z |
|  | logs-30-s1000-q1 | logs/30/s1000 | lookup | What is the latency_ms of the log entry with trace_id 969de62ca2f07c35? | 1224 | ✓ · 1886 · 108 (96) · 2.1s · 1224 | ✓ · 1525 · 98 (86) · 1.4s · 1224 |
|  | logs-30-s1000-q2 | logs/30/s1000 | count | How many ERROR-level entries are from service "billing"? | 2 | ✓ · 1874 · 223 (212) · 4.9s · 2 | ✓ · 1513 · 608 (597) · 8.9s · 2 |
|  | logs-30-s1000-q3 | logs/30/s1000 | argmax | Which trace_id has the highest latency_ms? | d4565997dad9e38e | ✓ · 1872 · 220 (201) · 5.0s · d4565997dad9e38e | ✓ · 1511 · 172 (153) · 4.5s · d4565997dad9e38e |
|  | logs-30-s1000-q4 | logs/30/s1000 | count | How many entries have status 500? | 5 | ✗ · 1871 · 165 (154) · 2.6s · 4 | ✗ · 1510 · 181 (170) · 2.8s · 6 |
|  | logs-30-s1000-q5 | logs/30/s1000 | lookup | Which service logged the entry at ts 2026-09-30T14:00:09.629Z? | worker | ✓ · 1887 · 74 (63) · 1.7s · worker | ✓ · 1526 · 97 (86) · 1.8s · worker |
|  | logs-30-s1000-q6 | logs/30/s1000 | lookup | What is the message of the entry with trace_id 2649d5d0fb51c38e? | payment authorized | ✓ · 1886 · 171 (159) · 5.3s · payment authorized | ✓ · 1525 · 96 (84) · 3.0s · payment authorized |
|  | logs-120-s1000-q1 | logs/120/s1000 | lookup | What is the latency_ms of the log entry with trace_id b8ee81b576a2453a? | 19 | ✓ · 6912 · 68 (57) · 2.3s · 19 | ✓ · 5164 · 96 (85) · 1.6s · 19 |
| ⚠ | logs-120-s1000-q3 | logs/120/s1000 | argmax | Which trace_id has the highest latency_ms? | 41c0eeb5f46155b9 | ✓ · 6898 · 131 (110) · 6.1s · 41c0eeb5f46155b9 | ✗ · 5150 · 98 (76) · 1.4s · 8b5b3f7aad413d6a |
|  | logs-120-s1000-q5 | logs/120/s1000 | lookup | Which service logged the entry at ts 2026-09-30T14:01:32.439Z? | worker | ✓ · 6913 · 99 (88) · 1.8s · worker | ✓ · 5165 · 100 (89) · 1.7s · worker |
|  | logs-120-s1000-q6 | logs/120/s1000 | lookup | What is the message of the entry with trace_id 1e67b72076daba22? | connection reset by peer | ✓ · 6910 · 115 (101) · 6.2s · connection reset by peer | ✓ · 5162 · 97 (83) · 6.4s · connection reset by peer |
|  | search-30-s1000-q1 | search/30/s1000 | lookup | What is the url of the result titled "Qudit leader index guide"? | https://docs.example.com/qudit-leader-in… | ✓ · 2422 · 78 (57) · 1.6s · https://docs.example.com/qudit-leader-in… | ✓ · 2422 · 93 (72) · 6.2s · https://docs.example.com/qudit-leader-in… |
|  | search-30-s1000-q2 | search/30/s1000 | lookup | What is the doc_id of the result with rank 5? | D-34053 | ✓ · 2420 · 62 (48) · 1.5s · D-34053 | ✓ · 2420 · 50 (36) · 1.1s · D-34053 |
|  | search-30-s1000-q3 | search/30/s1000 | lookup | What is the score of document D-35443? | 0.3204 | ✓ · 2418 · 66 (52) · 1.5s · 0.3204 | ✓ · 2418 · 83 (69) · 1.8s · 0.3204 |
|  | search-30-s1000-q4 | search/30/s1000 | count | How many results have a url on the domain blog.example.org? | 4 | ✓ · 2420 · 326 (315) · 14.0s · 4 | ✓ · 2420 · 131 (120) · 2.8s · 4 |
|  | logs-120-s1000-q4 | logs/120/s1000 | count | How many entries have status 500? | 12 | ✓ · 6897 · 7845 (7834) · 127.9s · 12 | ✓ · 5149 · 2371 (2360) · 43.6s · 12 |
| ⚠ | logs-120-s1000-q2 | logs/120/s1000 | count | How many ERROR-level entries are from service "auth"? | 3 | ✗ · 6900 · 16384 (16381) · 256.0s ·  | ✓ · 5152 · 4518 (4507) · 99.7s · 3 |
|  | search-120-s1000-q1 | search/120/s1000 | lookup | What is the url of the result titled "Rpan latency cluster guide"? | https://blog.example.org/rpan-latency-cl… | ✓ · 9022 · 67 (44) · 1.2s · https://blog.example.org/rpan-latency-cl… | ✓ · 7677 · 79 (56) · 4.6s · https://blog.example.org/rpan-latency-cl… |
|  | search-120-s1000-q2 | search/120/s1000 | lookup | What is the doc_id of the result with rank 27? | D-10132 | ✓ · 9020 · 49 (35) · 1.3s · D-10132 | ✓ · 7675 · 47 (33) · 1.3s · D-10132 |
|  | search-120-s1000-q3 | search/120/s1000 | lookup | What is the score of document D-20616? | 0.3069 | ✓ · 9018 · 49 (35) · 1.2s · 0.3069 | ✓ · 7673 · 49 (35) · 2.7s · 0.3069 |
|  | employees-30-s1000-q1 | employees/30/s1000 | lookup | What is the email of employee id 5009? | chen.tanaka@example.com | ✓ · 2319 · 46 (30) · 1.3s · chen.tanaka@example.com | ✓ · 1772 · 51 (35) · 2.4s · chen.tanaka@example.com |
|  | employees-30-s1000-q2 | employees/30/s1000 | count | How many active employees are in department "Support" and located in Berlin? | 1 | ✓ · 2323 · 305 (294) · 6.0s · 1 | ✓ · 1776 · 9759 (9734) · 176.7s · 1 |
|  | employees-30-s1000-q4 | employees/30/s1000 | lookup | What is the manager_id of Quinn Kowalski? | 5006 | ✓ · 2319 · 48 (36) · 1.3s · 5006 | ✓ · 1772 · 55 (43) · 1.5s · 5006 |
|  | employees-30-s1000-q5 | employees/30/s1000 | count | How many employees have level L5? | 3 | ✓ · 2316 · 72 (61) · 2.1s · 3 | ✓ · 1769 · 199 (188) · 4.9s · 3 |
| ⚠ | search-120-s1000-q4 | search/120/s1000 | count | How many results have a url on the domain wiki.internal? | 21 | ✓ · 9019 · 1315 (1304) · 24.5s · 21 | ✗ · 7674 · 3282 (3271) · 39.3s · 7 |
|  | employees-30-s1000-q6 | employees/30/s1000 | lookup | What is the start_date of employee id 5025? | 2024-07-19 | ✓ · 2320 · 84 (68) · 2.7s · 2024-07-19 | ✓ · 1773 · 82 (66) · 1.3s · 2024-07-19 |
|  | employees-120-s1000-q1 | employees/120/s1000 | lookup | What is the email of employee id 5087? | hana.castillo@example.com | ✓ · 8634 · 48 (33) · 1.1s · hana.castillo@example.com | ✓ · 5983 · 53 (38) · 0.7s · hana.castillo@example.com |
|  | employees-30-s1000-q3 | employees/30/s1000 | argmax | Which employee in department "Engineering" has the highest salary? Answer with their first… | Rosa Duarte | ✓ · 2328 · 4561 (4547) · 66.0s · Rosa Duarte | ✓ · 1781 · 787 (774) · 29.0s · Rosa Duarte |
|  | employees-120-s1000-q2 | employees/120/s1000 | count | How many active employees are in department "Marketing" and located in Bangalore? | 7 | ✗ · 8638 · 643 (632) · 12.4s · 6 | ✗ · 5987 · 1333 (1322) · 41.2s · 11 |
| ⚠ | employees-120-s1000-q4 | employees/120/s1000 | lookup | What is the manager_id of Goran Sato? | 5008 | ✓ · 8634 · 120 (108) · 2.1s · 5008 | ✗ · 5983 · 175 (163) · 1.6s · 5016 |
| ⚠ | employees-120-s1000-q3 | employees/120/s1000 | argmax | Which employee in department "Marketing" has the highest salary? Answer with their first a… | Daria Rossi | ✓ · 8643 · 3069 (3056) · 27.9s · Daria Rossi | ✗ · 5992 · 2405 (2390) · 21.8s · Daria Mbeki |
|  | employees-120-s1000-q6 | employees/120/s1000 | lookup | What is the start_date of employee id 5028? | 2023-05-21 | ✓ · 8635 · 48 (32) · 1.8s · 2023-05-21 | ✓ · 5984 · 193 (177) · 2.2s · 2023-05-21 |
|  | orders-30-s1001-q1 | orders/30/s1001 | lookup | What is the customer name on order id 10026? | Vandelay Corp | ✓ · 1644 · 76 (62) · 1.0s · Vandelay Corp | ✓ · 1340 · 84 (70) · 2.0s · Vandelay Corp |
|  | employees-120-s1000-q5 | employees/120/s1000 | count | How many employees have level L1? | 21 | ✓ · 8631 · 826 (815) · 314.9s · 21 | ✓ · 5980 · 580 (569) · 13.0s · 21 |
|  | orders-30-s1001-q2 | orders/30/s1001 | lookup | What is the amount of order id 10007? | 1695.05 | ✓ · 1643 · 147 (133) · 3.5s · 1695.05 | ✓ · 1339 · 78 (64) · 1.3s · 1695.05 |
|  | orders-30-s1001-q3 | orders/30/s1001 | count | How many orders have status "pending"? | 7 | ✓ · 1640 · 133 (122) · 1.7s · 7 | ✓ · 1336 · 233 (222) · 2.7s · 7 |
|  | orders-30-s1001-q5 | orders/30/s1001 | argmax | Which order id has the largest amount? | 10021 | ✓ · 1640 · 91 (79) · 2.8s · 10021 | ✓ · 1336 · 205 (193) · 2.8s · 10021 |
|  | orders-30-s1001-q4 | orders/30/s1001 | count | How many orders have status "paid" and currency "GBP"? | 4 | ✓ · 1645 · 255 (244) · 7.5s · 4 | ✓ · 1341 · 2585 (2574) · 90.7s · 4 |
|  | orders-30-s1001-q6 | orders/30/s1001 | sum | What is the total number of items across all orders from customer "Aperture Industries"? | 15 | ✓ · 1650 · 105 (94) · 1.5s · 15 | ✓ · 1346 · 424 (413) · 4.8s · 15 |
|  | orders-30-s1001-q7 | orders/30/s1001 | lookup | What is the created_at timestamp of order id 10012? | 2026-03-06T06:27:00Z | ✓ · 1645 · 65 (42) · 1.6s · 2026-03-06T06:27:00Z | ✓ · 1341 · 98 (75) · 1.2s · 2026-03-06T06:27:00Z |
|  | orders-120-s1001-q1 | orders/120/s1001 | lookup | What is the customer name on order id 10041? | Hooli Labs | ✓ · 5868 · 46 (33) · 1.1s · Hooli Labs | ✓ · 4397 · 77 (64) · 1.8s · Hooli Labs |
|  | orders-120-s1001-q2 | orders/120/s1001 | lookup | What is the amount of order id 10043? | 51.95 | ✓ · 5867 · 42 (29) · 1.0s · 51.95 | ✓ · 4396 · 84 (71) · 2.2s · 51.95 |
|  | orders-120-s1001-q4 | orders/120/s1001 | count | How many orders have status "paid" and currency "EUR"? | 7 | ✓ · 5869 · 910 (899) · 23.0s · 7 | ✓ · 4398 · 4850 (4839) · 53.6s · 7 |
|  | orders-120-s1001-q3 | orders/120/s1001 | count | How many orders have status "pending"? | 31 | ✓ · 5864 · 1128 (1117) · 36.7s · 31 | ✓ · 4393 · 1808 (1797) · 68.4s · 31 |
|  | orders-120-s1001-q5 | orders/120/s1001 | argmax | Which order id has the largest amount? | 10029 | ✓ · 5864 · 3571 (3559) · 62.8s · 10029 | ✓ · 4393 · 167 (155) · 3.2s · 10029 |
|  | orders-120-s1001-q6 | orders/120/s1001 | sum | What is the total number of items across all orders from customer "Hooli Labs"? | 30 | ✓ · 5873 · 175 (164) · 2.3s · 30 | ✓ · 4402 · 251 (240) · 9.8s · 30 |
|  | orders-120-s1001-q7 | orders/120/s1001 | lookup | What is the created_at timestamp of order id 10108? | 2026-05-29T16:51:00Z | ✓ · 5869 · 67 (44) · 1.2s · 2026-05-29T16:51:00Z | ✓ · 4398 · 100 (77) · 1.3s · 2026-05-29T16:51:00Z |
|  | logs-30-s1001-q1 | logs/30/s1001 | lookup | What is the latency_ms of the log entry with trace_id 841272c203b08f04? | 2871 | ✓ · 1901 · 146 (134) · 6.6s · 2871 | ✓ · 1542 · 101 (89) · 1.9s · 2871 |
|  | logs-30-s1001-q2 | logs/30/s1001 | count | How many ERROR-level entries are from service "worker"? | 2 | ✓ · 1890 · 2208 (2197) · 49.9s · 2 | ✓ · 1531 · 331 (320) · 2.6s · 2 |
|  | logs-30-s1001-q3 | logs/30/s1001 | argmax | Which trace_id has the highest latency_ms? | b090ea24c5b5fae1 | ✓ · 1888 · 141 (120) · 2.4s · b090ea24c5b5fae1 | ✓ · 1529 · 251 (230) · 7.2s · b090ea24c5b5fae1 |
| ⚠ | logs-30-s1001-q4 | logs/30/s1001 | count | How many entries have status 500? | 4 | ✗ · 1887 · 174 (163) · 5.4s · 5 | ✓ · 1528 · 297 (286) · 5.3s · 4 |
|  | logs-30-s1001-q5 | logs/30/s1001 | lookup | Which service logged the entry at ts 2026-09-30T14:00:12.680Z? | worker | ✓ · 1903 · 76 (65) · 3.4s · worker | ✓ · 1544 · 139 (128) · 4.2s · worker |
|  | logs-30-s1001-q6 | logs/30/s1001 | lookup | What is the message of the entry with trace_id b42fc58a4cfe4e02? | invalid signature | ✓ · 1901 · 166 (154) · 2.7s · invalid signature | ✓ · 1542 · 94 (82) · 2.8s · invalid signature |
|  | logs-120-s1001-q1 | logs/120/s1001 | lookup | What is the latency_ms of the log entry with trace_id ec531f66e8214cd4? | 2179 | ✓ · 6899 · 56 (44) · 1.2s · 2179 | ✓ · 5158 · 88 (76) · 2.4s · 2179 |
|  | logs-120-s1001-q3 | logs/120/s1001 | argmax | Which trace_id has the highest latency_ms? | 8d325a771666a2d1 | ✓ · 6886 · 154 (134) · 8.3s · 8d325a771666a2d1 | ✓ · 5145 · 548 (528) · 29.8s · 8d325a771666a2d1 |
|  | logs-120-s1001-q4 | logs/120/s1001 | count | How many entries have status 500? | 6 | ✓ · 6885 · 10205 (10194) · 206.8s · 6 | ✓ · 5144 · 1500 (1489) · 26.7s · 6 |
|  | logs-120-s1001-q5 | logs/120/s1001 | lookup | Which service logged the entry at ts 2026-09-30T14:02:39.834Z? | auth | ✓ · 6901 · 133 (122) · 3.1s · auth | ✓ · 5160 · 90 (79) · 3.5s · auth |
|  | logs-120-s1001-q6 | logs/120/s1001 | lookup | What is the message of the entry with trace_id eea3555747c87e4d? | connection reset by peer | ✓ · 6898 · 55 (41) · 1.8s · connection reset by peer | ✓ · 5157 · 97 (83) · 3.6s · connection reset by peer |
|  | search-30-s1001-q1 | search/30/s1001 | lookup | What is the url of the result titled "Sompaction region shard guide"? | https://docs.example.com/sompaction-regi… | ✓ · 2397 · 81 (59) · 1.7s · https://docs.example.com/sompaction-regi… | ✓ · 2397 · 65 (43) · 3.7s · https://docs.example.com/sompaction-regi… |
|  | search-30-s1001-q2 | search/30/s1001 | lookup | What is the doc_id of the result with rank 13? | D-03854 | ✓ · 2394 · 50 (36) · 1.2s · D-03854 | ✓ · 2394 · 50 (36) · 1.0s · D-03854 |
|  | search-30-s1001-q3 | search/30/s1001 | lookup | What is the score of document D-89164? | 0.2157 | ✓ · 2392 · 44 (30) · 1.7s · 0.2157 | ✓ · 2392 · 44 (30) · 1.8s · 0.2157 |
|  | search-30-s1001-q4 | search/30/s1001 | count | How many results have a url on the domain kb.example.net? | 6 | ✓ · 2394 · 235 (224) · 4.3s · 6 | ✓ · 2394 · 1429 (1418) · 14.1s · 6 |
|  | logs-120-s1001-q2 | logs/120/s1001 | count | How many ERROR-level entries are from service "worker"? | 6 | ✓ · 6888 · 3134 (3123) · 112.7s · 6 | ✓ · 5147 · 9166 (9155) · 204.7s · 6 |
|  | search-120-s1001-q1 | search/120/s1001 | lookup | What is the url of the result titled "Martition query router guide"? | https://support.example.com/martition-qu… | ✓ · 9005 · 76 (55) · 4.6s · https://support.example.com/martition-qu… | ✓ · 7660 · 73 (52) · 2.3s · https://support.example.com/martition-qu… |
|  | search-120-s1001-q2 | search/120/s1001 | lookup | What is the doc_id of the result with rank 115? | D-73360 | ✓ · 9003 · 70 (56) · 2.4s · D-73360 | ✓ · 7658 · 47 (33) · 2.3s · D-73360 |
|  | search-120-s1001-q3 | search/120/s1001 | lookup | What is the score of document D-72627? | 0.316 | ✓ · 9001 · 46 (33) · 1.1s · 0.316 | ✓ · 7656 · 49 (36) · 1.2s · 0.316 |
| ⚠ | search-120-s1001-q4 | search/120/s1001 | count | How many results have a url on the domain docs.example.com? | 30 | ✓ · 9003 · 1741 (1730) · 20.3s · 30 | ✗ · 7658 · 768 (757) · 25.5s · 16 |
|  | employees-30-s1001-q1 | employees/30/s1001 | lookup | What is the email of employee id 5014? | uma.brennan@example.com | ✓ · 2307 · 47 (31) · 0.9s · uma.brennan@example.com | ✓ · 1768 · 58 (42) · 2.6s · uma.brennan@example.com |
|  | employees-30-s1001-q2 | employees/30/s1001 | count | How many active employees are in department "Sales" and located in Bangalore? | 0 | ✓ · 2311 · 74 (63) · 2.2s · 0 | ✓ · 1772 · 212 (201) · 7.2s · 0 |
|  | employees-30-s1001-q4 | employees/30/s1001 | lookup | What is the manager_id of Kofi Novak? | 5007 | ✓ · 2306 · 46 (34) · 1.9s · 5007 | ✓ · 1767 · 67 (55) · 3.7s · 5007 |
| ⚠ | employees-30-s1001-q3 | employees/30/s1001 | argmax | Which employee in department "Engineering" has the highest salary? Answer with their first… | Tara Duarte | ✓ · 2316 · 241 (228) · 8.5s · Tara Duarte | ✗ · 1777 · 437 (421) · 4.7s · Viktor Kowalski |
| ⚠ | employees-30-s1001-q6 | employees/30/s1001 | lookup | What is the start_date of employee id 5005? | 2015-07-12 | ✓ · 2308 · 82 (66) · 2.4s · 2015-07-12 | ✗ · 1769 · 61 (45) · 2.5s · 2024-07-27 |
|  | employees-30-s1001-q5 | employees/30/s1001 | count | How many employees have level L3? | 4 | ✓ · 2304 · 284 (273) · 10.2s · 4 | ✓ · 1765 · 213 (202) · 5.6s · 4 |
|  | employees-120-s1001-q1 | employees/120/s1001 | lookup | What is the email of employee id 5019? | ivan.haddad@example.com | ✓ · 8587 · 46 (30) · 2.0s · ivan.haddad@example.com | ✓ · 5960 · 52 (36) · 2.9s · ivan.haddad@example.com |
|  | employees-120-s1001-q4 | employees/120/s1001 | lookup | What is the manager_id of Hana Brennan? | 5031 | ✓ · 8585 · 60 (48) · 2.7s · 5031 | ✓ · 5958 · 297 (285) · 12.1s · 5031 |
| ⚠ | employees-120-s1001-q3 | employees/120/s1001 | argmax | Which employee in department "Sales" has the highest salary? Answer with their first and l… | Goran Castillo | ✓ · 8596 · 2676 (2663) · 42.3s · Goran Castillo | ✗ · 5969 · 2917 (2903) · 170.8s · Jia Iyer |
|  | employees-120-s1001-q5 | employees/120/s1001 | count | How many employees have level L4? | 26 | ✓ · 8584 · 953 (942) · 11.3s · 26 | ✓ · 5957 · 668 (657) · 12.6s · 26 |
| ⚠ | employees-120-s1001-q6 | employees/120/s1001 | lookup | What is the start_date of employee id 5007? | 2019-08-13 | ✓ · 8588 · 48 (32) · 2.9s · 2019-08-13 | ✗ · 5961 · 268 (252) · 7.6s · 2021-08-04 |
|  | orders-30-s1002-q1 | orders/30/s1002 | lookup | What is the customer name on order id 10026? | Oscorp Partners | ✓ · 1634 · 46 (33) · 0.9s · Oscorp Partners | ✓ · 1336 · 76 (63) · 4.7s · Oscorp Partners |
|  | orders-30-s1002-q2 | orders/30/s1002 | lookup | What is the amount of order id 10027? | 1558.54 | ✓ · 1633 · 45 (31) · 1.3s · 1558.54 | ✓ · 1335 · 80 (66) · 3.9s · 1558.54 |
|  | orders-30-s1002-q4 | orders/30/s1002 | count | How many orders have status "paid" and currency "GBP"? | 3 | ✓ · 1635 · 218 (207) · 3.6s · 3 | ✓ · 1337 · 980 (969) · 7.7s · 3 |
|  | orders-30-s1002-q5 | orders/30/s1002 | argmax | Which order id has the largest amount? | 10024 | ✓ · 1630 · 242 (230) · 3.4s · 10024 | ✓ · 1332 · 245 (233) · 3.7s · 10024 |
|  | orders-30-s1002-q3 | orders/30/s1002 | count | How many orders have status "refunded"? | 10 | ✓ · 1631 · 355 (344) · 25.3s · 10 | ✓ · 1333 · 243 (232) · 16.8s · 10 |
|  | orders-30-s1002-q6 | orders/30/s1002 | sum | What is the total number of items across all orders from customer "Hooli Corp"? | 23 | ✓ · 1639 · 142 (131) · 2.4s · 23 | ✓ · 1341 · 437 (426) · 17.5s · 23 |
|  | orders-30-s1002-q7 | orders/30/s1002 | lookup | What is the created_at timestamp of order id 10006? | 2026-05-27T22:07:00Z | ✓ · 1635 · 65 (42) · 4.9s · 2026-05-27T22:07:00Z | ✓ · 1337 · 100 (77) · 1.4s · 2026-05-27T22:07:00Z |
|  | orders-120-s1002-q1 | orders/120/s1002 | lookup | What is the customer name on order id 10021? | Monarch Partners | ✓ · 5901 · 100 (87) · 3.5s · Monarch Partners | ✓ · 4425 · 79 (66) · 2.0s · Monarch Partners |
|  | employees-120-s1001-q2 | employees/120/s1001 | count | How many active employees are in department "Engineering" and located in Toronto? | 6 | ✓ · 8591 · 2457 (2446) · 143.1s · 6 | ✓ · 5964 · 8863 (8852) · 99.2s · 6 |
|  | orders-120-s1002-q2 | orders/120/s1002 | lookup | What is the amount of order id 10117? | 563.59 | ✓ · 5900 · 94 (81) · 2.2s · 563.59 | ✓ · 4424 · 75 (62) · 2.6s · 563.59 |
|  | orders-120-s1002-q3 | orders/120/s1002 | count | How many orders have status "refunded"? | 36 | ✓ · 5898 · 598 (587) · 37.6s · 36 | ✓ · 4422 · 2245 (2234) · 127.2s · 36 |
|  | orders-120-s1002-q5 | orders/120/s1002 | argmax | Which order id has the largest amount? | 10038 | ✓ · 5897 · 173 (161) · 1.7s · 10038 | ✓ · 4421 · 2307 (2295) · 116.3s · 10038 |
|  | orders-120-s1002-q4 | orders/120/s1002 | count | How many orders have status "paid" and currency "GBP"? | 8 | ✓ · 5902 · 655 (644) · 36.3s · 8 | ✓ · 4426 · 1444 (1433) · 39.4s · 8 |
|  | orders-120-s1002-q7 | orders/120/s1002 | lookup | What is the created_at timestamp of order id 10023? | 2026-04-14T23:48:00Z | ✓ · 5902 · 117 (94) · 4.0s · 2026-04-14T23:48:00Z | ✓ · 4426 · 101 (78) · 2.9s · 2026-04-14T23:48:00Z |
|  | logs-30-s1002-q1 | logs/30/s1002 | lookup | What is the latency_ms of the log entry with trace_id 184a55f4ade7c790? | 2190 | ✓ · 1904 · 70 (58) · 4.9s · 2190 | ✓ · 1544 · 128 (116) · 8.2s · 2190 |
|  | logs-30-s1002-q2 | logs/30/s1002 | count | How many ERROR-level entries are from service "billing"? | 1 | ✓ · 1892 · 388 (377) · 15.2s · 1 | ✓ · 1532 · 3047 (3036) · 125.6s · 1 |
|  | logs-30-s1002-q3 | logs/30/s1002 | argmax | Which trace_id has the highest latency_ms? | e6d29863b5780e6c | ✓ · 1890 · 101 (80) · 3.9s · e6d29863b5780e6c | ✓ · 1530 · 152 (131) · 2.1s · e6d29863b5780e6c |
|  | logs-30-s1002-q4 | logs/30/s1002 | count | How many entries have status 500? | 1 | ✓ · 1889 · 307 (296) · 14.2s · 1 | ✓ · 1529 · 194 (183) · 3.1s · 1 |
|  | logs-30-s1002-q5 | logs/30/s1002 | lookup | Which service logged the entry at ts 2026-09-30T14:00:50.645Z? | api | ✓ · 1905 · 142 (131) · 7.2s · api | ✓ · 1545 · 120 (109) · 7.8s · api |
|  | logs-30-s1002-q6 | logs/30/s1002 | lookup | What is the message of the entry with trace_id 56a4cf8baad8c5ad? | rate limited by upstream | ✓ · 1904 · 239 (225) · 7.1s · rate limited by upstream | ✓ · 1544 · 74 (60) · 0.9s · rate limited by upstream |
|  | logs-120-s1002-q1 | logs/120/s1002 | lookup | What is the latency_ms of the log entry with trace_id f5d5e4a59fde9139? | 2911 | ✓ · 6930 · 137 (125) · 4.9s · 2911 | ✓ · 5192 · 134 (122) · 1.6s · 2911 |
|  | orders-120-s1002-q6 | orders/120/s1002 | sum | What is the total number of items across all orders from customer "Umbrella Holdings"? | 19 | ✓ · 5906 · 3249 (3238) · 150.7s · 19 | ✓ · 4430 · 5678 (5666) · 165.8s · 19 |
| ⚠ | logs-120-s1002-q3 | logs/120/s1002 | argmax | Which trace_id has the highest latency_ms? | d807fb58ef8e6f55 | ✓ · 6915 · 2515 (2495) · 59.0s · d807fb58ef8e6f55 | ✗ · 5177 · 1216 (1198) · 29.3s · 8ed09dd193a93fab |
|  | logs-120-s1002-q5 | logs/120/s1002 | lookup | Which service logged the entry at ts 2026-09-30T14:00:46.662Z? | search | ✓ · 6930 · 107 (96) · 2.1s · search | ✓ · 5192 · 131 (120) · 4.9s · search |
| ⚠ | logs-120-s1002-q4 | logs/120/s1002 | count | How many entries have status 500? | 10 | ✗ · 6914 · 5537 (5526) · 404.9s · 8 | ✓ · 5176 · 916 (905) · 21.2s · 10 |
|  | logs-120-s1002-q6 | logs/120/s1002 | lookup | What is the message of the entry with trace_id 9d59d8e2eaec2579? | cache miss | ✓ · 6929 · 111 (99) · 2.3s · cache miss | ✓ · 5191 · 90 (78) · 2.6s · cache miss |
|  | search-30-s1002-q1 | search/30/s1002 | lookup | What is the url of the result titled "Whard worker cache guide"? | https://wiki.internal/whard-worker-cache… | ✓ · 2389 · 70 (50) · 2.2s · https://wiki.internal/whard-worker-cache… | ✓ · 2389 · 71 (51) · 3.2s · https://wiki.internal/whard-worker-cache… |
|  | search-30-s1002-q2 | search/30/s1002 | lookup | What is the doc_id of the result with rank 15? | D-89779 | ✓ · 2387 · 46 (32) · 1.9s · D-89779 | ✓ · 2387 · 46 (32) · 0.9s · D-89779 |
|  | logs-120-s1002-q2 | logs/120/s1002 | count | How many ERROR-level entries are from service "api"? | 5 | ✓ · 6917 · 7572 (7561) · 144.7s · 5 | ✓ · 5179 · 7834 (7811) · 126.4s · 5 |

