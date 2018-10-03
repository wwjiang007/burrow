pragma solidity >=0.0.0;

contract Structs8 {
    struct Thing {
        address thingMaker;
        string description;
        string url;
        bytes32 filehash;
        bytes32 filename;
    }

    Thing[] things;

    function addThing(string description, string url, bytes32 filehash, bytes32 filename) public returns (uint) {
        things.push(Thing(msg.sender, description, url, filehash, filename));
        return 10;
    }
}