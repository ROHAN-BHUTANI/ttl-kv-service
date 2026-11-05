# Day 2: Go and coding-round practice (private notes, delete before pushing if you like)

## Go (about 3 hours)
1. Work through https://go.dev/tour/ (basics, methods, interfaces, concurrency).
2. Read this repo end to end. Be able to explain, without notes: why RWMutex, why values are copied, how the janitor stops, why readiness fails before shutdown.
3. Add one feature yourself: a `GET /v1/keys` endpoint that lists live keys, with a test. Then add one of the "Next steps" in the README. Only list on your resume what you built and can explain.

## Coding round (about 3 hours)
Rubrik-style online assessments are timed DSA problems. Do 6 to 8 mediums, 30 minutes each, no hints:
- Graphs: BFS/DFS, shortest path, topological sort
- Dynamic programming: knapsack-style, grid paths
- Heaps and intervals, sliding window, binary search on answer
- Write the brute force first, state its complexity, then optimise.

## Interview talking points for this project
- What breaks with 3 replicas, and how would you fix it?
- What happens to in-flight requests when a pod is terminated?
- Liveness versus readiness: why two endpoints?
- Where is the lock contention, and how would you measure it?
