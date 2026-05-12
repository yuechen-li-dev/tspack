import { Suite, Fact } from '../../../src/native-test';

let executed = false;

export function wasExecuted(): boolean {
  return executed;
}

export default (
  <Suite name="side">
    <Fact name="body">
      {() => {
        executed = true;
      }}
    </Fact>
  </Suite>
);
