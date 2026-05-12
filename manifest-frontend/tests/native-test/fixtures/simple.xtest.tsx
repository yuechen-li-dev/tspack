import { Suite, Fact, Theory, Case, assert, expect } from '../../../src/native-test';

export default (
  <Suite name="math">
    <Fact name="addition works">
      {() => {
        assert.equal(
          1 + 1,
          2,
          'addition should work inside a loaded xTest file',
        );

        expect(2 + 2)
          .toBe(4)
          .because('expect chains should finalize in loaded xTest files');
      }}
    </Fact>

    <Theory name="string lengths">
      <Case input="a" expected={1} />
      <Case input="abc" expected={3} />

      {({ input, expected }: { input: string; expected: number }) => {
        assert.equal(
          input.length,
          expected,
          'theory case input length should match expected value',
        );
      }}
    </Theory>
  </Suite>
);
