import { assert, Case, Fact, skip, Suite, Theory } from '../../../src/native-test';

export default (
  <Suite name="skip">
    <Fact name="conditional skip">
      {() => {
        if (true) {
          skip('demonstrates runtime conditional skip');
        }

        assert.fail('skip should stop execution before this assertion');
      }}
    </Fact>

    <Theory name="case skip">
      <Case value={1} />
      <Case value={2} />

      {({ value }: { value: number }) => {
        if (value === 1) {
          skip('case 1 intentionally skipped');
        }

        assert.equal(
          value,
          2,
          'case 2 should still run after case 1 is skipped',
        );
      }}
    </Theory>
  </Suite>
);
