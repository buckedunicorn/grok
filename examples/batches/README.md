# batches

Batch job creation, request addition, and cancellation.

## Run

```sh
XAI_API_KEY=... go run ./examples/batches
```

## What it shows

- Creating a batch with `Batches.Create`
- Adding multiple requests with `Batches.AddRequests` using `BatchRequestPayload`
- Cancelling a batch with `Batches.Cancel`

In a production workflow you would submit the batch, wait for `state.NumPending == 0`, then call `Batches.GetResults` to retrieve outputs.
