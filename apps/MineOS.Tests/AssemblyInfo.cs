using Xunit;

// xUnit runs test collections in parallel by default. ServerService guards every
// start/stop/restart with a per-server SemaphoreSlim that gives up after five
// seconds and reports the server as busy — correct in production, where a wait
// that long means something really is stuck.
//
// Under a parallel run that timeout measures thread-pool contention rather than
// the gate: ServerStartConcurrencyTests parks a caller inside the gate on
// purpose, and once the suite is wide enough the waiting caller's continuation
// is scheduled later than five seconds and the test fails without anything
// being wrong. It passed at upstream's 277 tests and fails at ours; the next
// batch of tests would have pushed upstream over the same edge.
//
// Serialising the collections costs roughly 25 seconds locally and makes the
// result depend on the code rather than on how busy the machine is. For a suite
// that gates a merge, that trade is worth it.
[assembly: CollectionBehavior(DisableTestParallelization = true)]
