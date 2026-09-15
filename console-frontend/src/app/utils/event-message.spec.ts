import formatEventMessage from './event-message';

describe('formatEventMessage', () => {
  it('leaves a plain message alone', () => {
    expect(formatEventMessage('Shoot spec applied')).toBe('Shoot spec applied');
  });

  it('reads the message and status code out of an escaped error body', () => {
    expect(
      formatEventMessage(
        'create shoot: {\\"statusCode\\":422,\\"message\\":\\"Invalid machine type\\"}',
      ),
    ).toBe('create shoot: Invalid machine type (status 422)');
  });

  it('matches keys regardless of case', () => {
    expect(formatEventMessage('{"statuscode":422,"Message":"Quota exceeded"}')).toBe(
      'Quota exceeded (status 422)',
    );
  });

  it('ignores a non-numeric Kubernetes status in favour of the code', () => {
    expect(
      formatEventMessage(
        'apply shoot: {"kind":"Status","status":"Failure","message":"shoot is invalid","reason":"Invalid","code":422}',
      ),
    ).toBe('apply shoot: shoot is invalid (status 422)');
  });

  it('finds a message nested under an error object', () => {
    expect(formatEventMessage('{"error":{"code":403,"message":"forbidden"}}')).toBe(
      'forbidden (status 403)',
    );
  });

  it('joins the messages of an errors array', () => {
    expect(
      formatEventMessage('{"statusCode":400,"errors":[{"message":"name taken"},"region unknown"]}'),
    ).toBe('name taken; region unknown (status 400)');
  });

  it('decodes a body that was JSON-encoded twice', () => {
    expect(formatEventMessage('"{\\"statusCode\\":500,\\"message\\":\\"boom\\"}"')).toBe(
      'boom (status 500)',
    );
  });

  it('does not repeat a message the error chain already ends with', () => {
    expect(
      formatEventMessage('sync: quota exceeded: {"code":429,"message":"quota exceeded"}'),
    ).toBe('sync: quota exceeded (status 429)');
  });

  it('only strips the escaping when the body is not valid JSON', () => {
    expect(formatEventMessage('bad body {\\"oops\\": }')).toBe('bad body {"oops": }');
  });

  it('keeps JSON without a message or code as it is', () => {
    expect(formatEventMessage('labels {"env":"prod"}')).toBe('labels {"env":"prod"}');
  });
});
