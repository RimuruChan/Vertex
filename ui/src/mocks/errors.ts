export class MockError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message)
  }
}
