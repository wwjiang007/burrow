pragma solidity >=0.0.0;

contract Structs2 {
    struct Thing {
        address thingMaker;
        string description;
        bytes32 url;
        bytes32 filehash;
        string filename;
    }

    Thing[] things;

    function addThing(string memory description, bytes32 url, bytes32 filehash, string memory filename) public returns (uint) {
        things.push(Thing(msg.sender, description, url, filehash, filename));
        return 10;
    }
}
