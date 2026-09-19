#!/usr/bin/env python3
"""Add only the public conversation SDK contract to a pinned public SDK checkout."""
import pathlib
import sys
root = pathlib.Path(sys.argv[1])
source = pathlib.Path(__file__).resolve().parent
proto = root / "proto/kandev/plugin/v1/plugin.proto"
text = proto.read_text()
anchor = "  rpc SendMessage(SendMessageRequest) returns (SendMessageResponse);"
if "rpc ResolveConversation(" not in text:
    if text.count(anchor) != 1:
        raise SystemExit("Unsupported SDK: SendMessage contract not found")
    text = text.replace(anchor, anchor + "\n  rpc ResolveConversation(ConversationTarget) returns (ConversationTarget);\n  rpc SubmitExternalMessage(ExternalMessageRequest) returns (ExternalMessageReceipt);\n  rpc GetExternalMessage(ExternalMessageLookup) returns (ExternalMessageReceipt);")
    text += "\n" + (source / "conversations.proto").read_text()
    proto.write_text(text)
(root / "pkg/pluginsdk/conversations.go").write_text((source / "conversations.go.txt").read_text())

text = proto.read_text()
if "rpc ResolveAttentionTarget(" not in text:
    text = text.replace("  rpc ResolveConversation(ConversationTarget) returns (ConversationTarget);", "  rpc ResolveConversation(ConversationTarget) returns (ConversationTarget);\n  rpc ResolveAttentionTarget(ConversationTarget) returns (ConversationTarget);")
    proto.write_text(text)
(root / "pkg/pluginsdk/attention.go").write_text((source / "attention.go.txt").read_text())

text = proto.read_text()
if "rpc ExternalAssistant(" not in text:
    text = text.replace(anchor, anchor + "\n  rpc ExternalAssistant(ExternalAssistantRequest) returns (ExternalAssistantResponse);")
    text += "\nmessage ExternalAssistantRequest { string operation = 1; string payload_json = 2; }\nmessage ExternalAssistantResponse { string payload_json = 1; }\n"
    proto.write_text(text)
(root / "pkg/pluginsdk/assistant.go").write_text((source / "assistant.go.txt").read_text())
