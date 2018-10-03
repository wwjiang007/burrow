pragma solidity >=0.0.0;

contract GSMulti {
  uint storedData1;
  uint storedData2;

  constructor(uint initialValue1, uint initialValue2) public {
    storedData1 = initialValue1;
    storedData2 = initialValue2;
  }

  function set(uint first, uint second) public {
    storedData1 = first;
    storedData2 = second;
  }

  function getFirst() public constant returns (uint retVal) {
    return storedData1;
  }

  function getSecond() public constant returns (uint retVal) {
    return storedData2;
  }
}
