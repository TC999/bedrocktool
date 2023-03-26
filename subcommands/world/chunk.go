package world

import (
	"github.com/bedrock-tool/bedrocktool/locale"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"github.com/sirupsen/logrus"
)

func (w *WorldState) processChangeDimension(pk *packet.ChangeDimension) {
	if len(w.chunks) > 0 {
		w.SaveAndReset()
	} else {
		logrus.Info(locale.Loc("not_saving_empty", nil))
		w.Reset()
	}
	dimensionID := pk.Dimension
	if w.ispre118 {
		dimensionID += 10
	}
	w.Dim = dimensionIDMap[uint8(dimensionID)]
}

func (w *WorldState) processLevelChunk(pk *packet.LevelChunk) {
	// ignore empty chunks THANKS WEIRD SERVER SOFTWARE DEVS
	if len(pk.RawPayload) == 0 {
		logrus.Info(locale.Loc("empty_chunk", nil))
		return
	}

	var subChunkCount int
	switch pk.SubChunkCount {
	case protocol.SubChunkRequestModeLimited:
		fallthrough
	case protocol.SubChunkRequestModeLimitless:
		subChunkCount = 0
	default:
		subChunkCount = int(pk.SubChunkCount)
	}

	ch, blockNBTs, err := chunk.NetworkDecode(world.AirRID(), pk.RawPayload, subChunkCount, w.Dim.Range(), w.ispre118, w.bp.HasBlocks())
	if err != nil {
		logrus.Error(err)
		return
	}
	if blockNBTs != nil {
		w.blockNBT[protocol.SubChunkPos{
			pk.Position.X(), 0, pk.Position.Z(),
		}] = blockNBTs
	}
	w.chunks[pk.Position] = ch

	max := w.Dim.Range().Height() / 16
	switch pk.SubChunkCount {
	case protocol.SubChunkRequestModeLimited:
		max = int(pk.HighestSubChunk)
		fallthrough
	case protocol.SubChunkRequestModeLimitless:
		var offsetTable []protocol.SubChunkOffset
		r := w.Dim.Range()
		for y := int8(r.Min() / 16); y < int8(r.Max()); y++ {
			offsetTable = append(offsetTable, protocol.SubChunkOffset{0, y, 0})
		}

		w.proxy.Server.WritePacket(&packet.SubChunkRequest{
			Dimension: int32(w.Dim.EncodeDimension()),
			Position: protocol.SubChunkPos{
				pk.Position.X(), 0, pk.Position.Z(),
			},
			Offsets: offsetTable[:max],
		})
	default:
		// legacy
		empty := true
		for _, sub := range ch.Sub() {
			if !sub.Empty() {
				empty = false
				break
			}
		}
		if !empty {
			w.mapUI.SetChunk(pk.Position, ch, true)
		}
	}
}

func (w *WorldState) processSubChunk(pk *packet.SubChunk) {
	posToRedraw := make(map[protocol.ChunkPos]bool)

	for _, sub := range pk.SubChunkEntries {
		var (
			absX   = pk.Position[0] + int32(sub.Offset[0])
			absY   = pk.Position[1] + int32(sub.Offset[1])
			absZ   = pk.Position[2] + int32(sub.Offset[2])
			subPos = protocol.SubChunkPos{absX, absY, absZ}
			pos    = protocol.ChunkPos{absX, absZ}
		)
		ch, ok := w.chunks[pos]
		if !ok {
			logrus.Error(locale.Loc("subchunk_before_chunk", nil))
			continue
		}
		blockNBT, err := ch.ApplySubChunkEntry(uint8(absY), &sub)
		if err != nil {
			logrus.Error(err)
		}
		if blockNBT != nil {
			w.blockNBT[subPos] = blockNBT
		}

		posToRedraw[pos] = true
	}

	// redraw the chunks
	for pos := range posToRedraw {
		w.mapUI.SetChunk(pos, w.chunks[pos], true)
	}
	w.mapUI.SchedRedraw()
}

func blockPosInChunk(pos protocol.BlockPos) (uint8, int16, uint8) {
	return uint8(pos.X() & 0x0f), int16(pos.Y() & 0x0f), uint8(pos.Z() & 0x0f)
}

func (w *WorldState) ProcessChunkPackets(pk packet.Packet) packet.Packet {
	switch pk := pk.(type) {
	case *packet.ChangeDimension:
		w.processChangeDimension(pk)
	case *packet.LevelChunk:
		w.processLevelChunk(pk)

		w.proxy.SendPopup(locale.Locm("popup_chunk_count", locale.Strmap{"Count": len(w.chunks), "Name": w.WorldName}, len(w.chunks)))
	case *packet.SubChunk:
		w.processSubChunk(pk)
	case *packet.BlockActorData:
		sp := protocol.SubChunkPos{pk.Position.X() << 4, 0, pk.Position.Z() << 4}
		b, ok := w.blockNBT[sp]
		if !ok {
			w.blockNBT[sp] = []map[string]any{pk.NBTData}
		} else {
			for i, v := range b {
				x, y, z := v["x"].(int32), v["y"].(int32), v["z"].(int32)
				if x == pk.Position.X() && y == pk.Position.Y() && z == pk.Position.Z() {
					b[i] = pk.NBTData
					break
				}
			}
		}
	case *packet.UpdateBlock:
		cp := protocol.ChunkPos{pk.Position.X() >> 4, pk.Position.Z() >> 4}
		c, ok := w.chunks[cp]
		if ok {
			x, y, z := blockPosInChunk(pk.Position)
			c.SetBlock(x, y, z, uint8(pk.Layer), pk.NewBlockRuntimeID)
			w.mapUI.SetChunk(cp, w.chunks[cp], true)
		}
	case *packet.UpdateSubChunkBlocks:
		cp := protocol.ChunkPos{pk.Position.X(), pk.Position.Z()}
		c, ok := w.chunks[cp]
		if ok {
			for _, bce := range pk.Blocks {
				x, y, z := blockPosInChunk(bce.BlockPos)
				if bce.SyncedUpdateType == packet.EntityToBlockTransition {
					c.SetBlock(x, y, z, 0, world.AirRID())
				} else {
					c.SetBlock(x, y, z, 0, bce.BlockRuntimeID)
				}
			}
			w.mapUI.SetChunk(cp, w.chunks[cp], true)
		}
	}
	return pk
}
