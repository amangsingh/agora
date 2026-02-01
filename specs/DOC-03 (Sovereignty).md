# DOC-03: The Foundry (agora-ui) & UX Specification v1.1

## 1\. Project Philosophy: The Joyful Canvas

* **Core Principle:** The Foundry is not a tool; it is an **instrument**. It is not a form; it is a **canvas**. Its primary directive is to be **intuitive, joyful, and frictionless**.  
* **User Experience Goal:** The user should feel like a composer, not a coder. The interface must get out of the way and let ideas flow directly from mind to screen.  
* **Core Metaphor:** A high-tech "Lego workshop" where users create bricks and then snap them together to build something magnificent.

---

## 2\. Core Workspaces

### 2.1. The Component Designer (The Universal Foundry)

This is the single, unified workspace for forging the fundamental building blocks of intelligence: `Tools` and `Recipes`.

#### 2.1.1. Forging `Tools`: The Visual Function Builder

This is a **visual programming environment** inspired by Lego and Node-RED, designed to make Go function creation intuitive and powerful.

* **Core UX:** Users drag-and-drop "logic blocks" onto a canvas to construct the flow of their function. These blocks are then connected to define the execution path.  
  * **Available Logic Blocks:** The standard library of blocks must include:  
  * **Variables:** Declare, Set, Get.  
  * **Control Flow:** If/Else, For Loop, While Loop.  
  * **Data Manipulation:** Struct creation, map operations, string formatting, basic math operations.  
  * **Networking:** Make HTTP Request (GET, POST, etc.).  
  * **File I/O:** Read File, Write File.  
  * **Logging:** Print to console.  
* **Two-Way Sync Code Editor:** A minimal, side-by-side Go code editor is present at all times.  
  * **Visual-to-Code:** As the user connects blocks on the canvas, idiomatic Go code is generated and displayed in the editor in real-time.  
  * **Code-to-Visual:** A user proficient in Go can type directly into the editor, and the visual block diagram will re-render itself to match the code's logic. This two-way binding is critical to serving both visual thinkers and expert coders.  
* **Output:** The visual design is serialized into the `functions.json` and `tools.json` blueprints for the Compiler.

#### 2.1.2. Forging `Recipes`

When a user selects "Create Recipe," the workspace transforms into a rich Markdown editor, optimized for writing clear, instructional prompts for an LLM.

### 2.2. The Skills Library (The Parts Warehouse)

A centralized repository to manage all reusable components.

* **UX:** A clean, searchable, list-based interface with tabs to filter between **`Tools`**, **`Recipes`**, and **`Appliances` (SubGraphs)**.  
* **Functionality:** Users can view, edit (which re-opens the component in its appropriate designer), or delete any component.

### 2.3. The Graph Designer (The Main Assembly Floor)

This is the primary canvas where the user assembles their agent by wiring together nodes. This is also where **`Appliances` are composed**.

* **UX:** A large, zoomable, pannable canvas (like Miro or Figma).  
* **Node Palette:** A left-hand sidebar allows users to drag-and-drop the standard nodes (`SimpleAgentNode`, `ToolAgentNode`, `ToolExecutorNode`) and any saved `Appliances` from the Skills Library.  
* **Appliance Composition:** A user can select a group of nodes on the canvas, right-click, and choose "Save as Appliance." This packages the subgraph, prompts for a name/description, and adds it to the Skills Library for reuse.

---

## 3\. The Golden Path (Revised User Journey)

1. **Idea:** A user wants to create an agent that can analyze text sentiment.  
2. **Forge a Tool:** They go to the **Component Designer**. Using the **visual function builder**, they drag blocks to create a `detectSentiment(text string) string` tool that calls an external sentiment analysis API. The Go code appears beside it in real-time.  
3. **Assemble the Graph:** They switch to the **Graph Designer** and wire together a `ToolAgentNode` and `ToolExecutorNode` to create a ReAct loop that uses the new tool.  
4. **Compose an Appliance:** They select this entire loop, right-click, and choose "Save as Appliance," naming it "SentimentAnalyzer."  
5. **Compile & Test:** The user clicks "Compile," and a sandboxed chat interface slides out, allowing immediate interaction with their newly created agent.

