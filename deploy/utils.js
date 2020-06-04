const ckb_utils = require("@nervosnetwork/ckb-sdk-utils");

function blake2b(buffer) {
    return ckb_utils
        .blake2b(32, null, null, ckb_utils.PERSONAL)
        .update(buffer)
        .digest("binary");
}

module.exports = {blake2b};