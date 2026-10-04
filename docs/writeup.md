# toonx cut tool-result input by 20–31%. trimproof's own test still didn't turn it on for any model we tested.

## The short version
- **Input:** on real public-API responses, toonx used 25–30% fewer input tokens than compact JSON, and 20–31% across every model and data set we tried.
- **Output:** the reasoning models often wrote more on toonx: up to +69% on synthetic tables (nemotron-3-ultra). With output priced at 4× input, the net saving ran from **−4.5% to +30.6%**, depending on the model and the data.
- **Accuracy:** it depended on the model. glm and nemotron-3-ultra were non-inferior within 3pp on two of three sets. gpt-oss was worse than JSON on synthetic tables (−7.8pp), and llama on both real payloads (−13.7pp) and synthetic tables (−12.2pp).
- **The decision:** we replayed every answer through trimproof's promotion test, the one a route runs in SHADOW mode. **It would not have turned toonx on for any model tested:**
  - llama and gpt-oss: switched **OFF**, confidently worse than the JSON-vs-JSON noise floor;
  - glm: still collecting after 469 samples, with the bounds straddling the margin;
  - nemotron-3-ultra: still collecting, and its 7% net saving is below the 15% minimum anyway.

That is the point of the project. A format that cuts input by a quarter can still cost accuracy on one model and money on another. Measure each route before switching.

## What was tested
- **Models:** four open models on NVIDIA NIM's free tier:
  - gpt-oss-20b;
  - nemotron-3-ultra;
  - glm-5.3-flash;
  - llama-3.2-90b-vision (the one non-reasoning model).
- **What wasn't:**
  - nemotron-3-super was retired by NVIDIA mid-run; its 76 questions are reported as partial.
  - The Mistral and Google slots had no available model.
  - No Claude or OpenAI-hosted model was tested.
- **Data:**
  - 139 questions over 36 real public-API responses;
  - 230 synthetic questions;
  - 100 WikiTableQuestions items (42 for llama, stopped early).
- **Arms:**
  - compact JSON;
  - the same JSON again, as the noise floor;
  - toonx, at 16,384 output tokens.
- **Pre-registered:** every rule was registered before its data. The 9 amendments and 1 deviation are public in [VERIFY-PLAN.md](https://github.com/AkashAgarwalInd/trimproof/blob/master/bench/results/VERIFY-PLAN.md). Every call is published: about 6,300 records, with the reasoning text where the model returned it.

## 1. Input savings are real

| model | payload Q&A | synthetic | WikiTableQuestions |
|---|---:|---:|---:|
| glm-5.3-flash | −26.5% [−29.1, −23.9] | −23.4% [−24.2, −22.5] | −30.4% [−34.7, −26.0] |
| llama-3.2-90b-vision | −30.4% [−32.9, −28.0] | −29.8% [−30.8, −28.8] | −30.9% [−35.9, −25.6] |
| gpt-oss-20b | −27.2% [−29.9, −24.7] | −23.3% [−24.2, −22.4] | −28.4% [−32.4, −24.1] |
| nemotron-3-ultra | −25.0% [−27.5, −22.6] | −20.4% [−21.0, −19.9] | −30.2% [−34.3, −26.0] |

Input tokens against compact JSON, with 95% bootstrap intervals.

## 2. Output decides the money

| model | payload Q&A | synthetic | WikiTableQuestions |
|---|---:|---:|---:|
| glm-5.3-flash | +27.1% | +17.3% | +25.6% |
| llama-3.2-90b-vision | +30.3% | +29.7% | +30.6% (partial) |
| gpt-oss-20b | +28.1% | +15.4% | −1.9% |
| nemotron-3-ultra | +25.3% | −4.2% | −4.5% |

Net saving with output at 4× input.
- **On real API responses, every model saved 25–30%.**
- **On synthetic and WikiTableQuestions, two reasoning models wrote enough extra output to lose money.** nemotron-3-ultra's output rose 69% on synthetic tables.
- **The pattern in the reasoning traces:** on wide tables, models find a value by counting columns, and that counting is where much of the extra output goes.

## 3. Accuracy, next to the noise floor

| model | payload Q&A | synthetic | WikiTableQuestions |
|---|---|---|---|
| glm-5.3-flash | non-inferior (−0.7pp) | non-inferior (+0.0pp) | inconclusive (−3.0pp) |
| nemotron-3-ultra | non-inferior (+0.0pp) | non-inferior (−0.4pp) | inconclusive (−8.0pp) |
| gpt-oss-20b | inconclusive (+2.2pp) | **worse** (−7.8pp) | inconclusive (−3.0pp) |
| llama-3.2-90b-vision | **worse** (−13.7pp) | **worse** (−12.2pp) | inconclusive (partial) |

Non-inferiority margin: 3pp. The JSON-vs-JSON noise floor for each cell is in [VERIFY.md](https://github.com/AkashAgarwalInd/trimproof/blob/master/bench/results/verify-2026-10/VERIFY.md).
- **gpt-oss's losses are concentrated on the 120-row tables.**
- **llama's are on both real payloads and synthetic tables,** while its two JSON runs scored identically.

## 4. Open-ended answers: not shown to be safe
- **All models:** a blind judge graded 104 free-form answers. toonx answers were judged wrong +5.8pp more often [−3.8, +15.4]. The 15pp bound was not met, so "no large drop" is not claimed.
- **glm:** its error share rose by +24.2pp [+6.1, +42.4], from wrong-row attribution, miscounts and dropped prefixes. So even the model with the best structured-answer results is not cleared for open-ended use.

## 5. The promotion replay

| model | samples | decided at | codec agreement | noise floor | net saved | state |
|---|---:|---:|---:|---:|---:|---|
| llama-3.2-90b-vision | 410 | 200 | 0.623 | 0.965 | 29.1% | **OFF** |
| gpt-oss-20b | 469 | 400 | 0.856 | 0.922 | 17.8% | **OFF** |
| glm-5.3-flash | 469 | – | 0.965 | 0.988 | 21.8% | SHADOW |
| nemotron-3-ultra | 469 | – | 0.958 | 0.983 | 7.1% | SHADOW |

- **How it was run:** with the gateway's own defaults (first look at 200 samples, then every 100, z = 2.5, margin 0.02, minimum net saving 15%), on every question all three arms answered, pooled per model.
- **What it shows:**
  - The test switched off the two models where toonx hurt.
  - It held glm, whose cost case is strong but whose agreement isn't yet proven within the margin.
  - It held nemotron-3-ultra, whose saving is too small to be worth it.

## 6. Multi-turn agents (computed, not measured)
- **The effect:** agents resend tool results on later turns, so the input saving repeats while the output difference is paid once.
- **Example:** in the projection, nemotron-3-super's payload net saving (partial, 76 questions) rises from +14.1% at 1 turn to +24.8% at 10.
- **With prompt caching,** resends are cheap and the advantage shrinks.
- No multi-turn run was made.

## What went wrong along the way
- **Pilot 2's rule fired.** A diagnostic showed the cause was the output limit, not the format.
- **Four codec versions were piloted.** The last change was chosen on small samples, so the full run measured toonx 3 frozen.
- **8 of the 36 payloads** would have been sent as JSON by the gateway under toonx 3's gates.
- **NVIDIA retired nemotron-3-super mid-run,** and nemotron-3-ultra replaced it. The −8.7pp `tabular` figure in the old README came from nemotron-3-super. On nemotron-3-ultra, `tabular` was non-inferior but cost 9.8% more.
- **We stopped llama's WikiTableQuestions stage early,** by choice, at 42 of 100 questions, after the machine running it slept overnight and after interim results had been seen. Its verdict was already settled on 368 other questions.
- **2 calls failed after the registered retry**, and were excluded.

## Reproduce it
Commands are in [VERIFY-PLAN.md](https://github.com/AkashAgarwalInd/trimproof/blob/master/bench/results/VERIFY-PLAN.md#amendment-8-the-full-run-before-any-result). Every table here is generated by `bench verify-report` and `bench promotion-replay` from the committed records.
